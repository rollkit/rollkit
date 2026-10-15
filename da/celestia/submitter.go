package celestia

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	keyring99 "github.com/99designs/keyring"
	"github.com/celestiaorg/celestia-app/v10/app"
	"github.com/celestiaorg/celestia-app/v10/app/encoding"
	"github.com/celestiaorg/celestia-app/v10/app/grpc/gasestimation"
	"github.com/celestiaorg/celestia-app/v10/pkg/appconsts"
	"github.com/celestiaorg/celestia-app/v10/pkg/user"
	blobtypes "github.com/celestiaorg/celestia-app/v10/x/blob/types"
	"github.com/celestiaorg/go-square/v4/share"
	"github.com/cosmos/cosmos-sdk/codec"
	"github.com/cosmos/cosmos-sdk/crypto/keyring"
	sdk "github.com/cosmos/cosmos-sdk/types"
	"github.com/rs/zerolog"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/metadata"

	"github.com/evstack/ev-node/pkg/config"
	blobrpc "github.com/evstack/ev-node/pkg/da/jsonrpc"
)

// PassphraseEnv holds the file-backend keyring passphrase; it takes precedence
// over tx_signer.passphrase_file.
const PassphraseEnv = "EVNODE_DA_KEYRING_PASSPHRASE" //nolint:gosec // env var name, not a credential

var errSignerAddress = errors.New("submit option signer_address is not supported with da.tx_signer; use key_name")

// Config configures the Submitter.
type Config struct {
	GRPCAddress       string
	GRPCAuthToken     string // sent as x-token metadata
	GRPCTLS           bool
	KeyringDir        string
	KeyringBackend    string
	KeyName           string
	KeyringPassphrase string //nolint:gosec // resolved at runtime, never logged
}

// ConfigFromNode maps da.tx_signer, resolving the keyring dir against the node
// home and reading the passphrase from the environment or passphrase_file.
func ConfigFromNode(nc config.Config) (Config, error) {
	t := nc.DA.TxSigner
	dir := t.KeyringDir
	if !filepath.IsAbs(dir) {
		dir = filepath.Join(nc.RootDir, dir)
	}

	var pass string
	if t.KeyringBackend == keyring.BackendFile {
		pass = os.Getenv(PassphraseEnv)
		if pass == "" && t.PassphraseFile != "" {
			b, err := os.ReadFile(t.PassphraseFile)
			if err != nil {
				return Config{}, fmt.Errorf("read keyring passphrase file: %w", err)
			}
			pass = strings.TrimSpace(string(b))
		}
		if pass == "" {
			return Config{}, fmt.Errorf("file keyring requires a passphrase via %s or tx_signer.passphrase_file", PassphraseEnv)
		}
	}

	if nc.DA.SubmitOptions != "" {
		var o blobrpc.SubmitOptions
		if err := json.Unmarshal([]byte(nc.DA.SubmitOptions), &o); err != nil {
			return Config{}, fmt.Errorf("parse da.submit_options: %w", err)
		}
		if o.SignerAddress != "" {
			return Config{}, errSignerAddress
		}
	}

	return Config{
		GRPCAddress:       t.GRPCAddress,
		GRPCAuthToken:     nc.DA.GRPCAuthTokenOrDefault(),
		GRPCTLS:           t.GRPCTLS,
		KeyringDir:        dir,
		KeyringBackend:    t.KeyringBackend,
		KeyName:           t.KeyName,
		KeyringPassphrase: pass,
	}, nil
}

type priceEstimator func(context.Context, gasestimation.TxPriority) (float64, error)

// Submitter implements block.BlobSubmitter with locally signed PayForBlobs.
type Submitter struct {
	tc      *user.TxClient
	conn    *grpc.ClientConn
	cancel  context.CancelFunc
	price   priceEstimator
	account string
}

// New opens the keyring, connects to consensus gRPC and checks the signing
// account exists on chain. The chain-id is taken from the node.
func New(ctx context.Context, cfg Config, logger zerolog.Logger) (*Submitter, error) {
	encCfg := encoding.MakeConfig(app.ModuleEncodingRegisters...)
	kr, err := openKeyring(cfg, encCfg.Codec)
	if err != nil {
		return nil, fmt.Errorf("open %s keyring at %s: %w", cfg.KeyringBackend, cfg.KeyringDir, err)
	}
	rec, err := kr.Key(cfg.KeyName)
	if err != nil {
		return nil, fmt.Errorf("load key %q from %s keyring at %s: %w", cfg.KeyName, cfg.KeyringBackend, cfg.KeyringDir, err)
	}
	addr, err := rec.GetAddress()
	if err != nil {
		return nil, err
	}

	conn, err := dial(cfg)
	if err != nil {
		return nil, fmt.Errorf("dial consensus gRPC %s: %w", cfg.GRPCAddress, err)
	}
	// SetupTxClient starts a queue worker that runs until ctx is cancelled.
	ctx, cancel := context.WithCancel(ctx)
	fail := func(err error) (*Submitter, error) {
		cancel()
		_ = conn.Close()
		return nil, err
	}
	if _, _, err := user.QueryAccount(ctx, conn, encCfg.InterfaceRegistry, addr); err != nil {
		return fail(fmt.Errorf("DA signer %s has no account on chain, fund it first: %w", addr, err))
	}

	tc, err := user.SetupTxClient(ctx, kr, conn, encCfg, user.WithDefaultAccount(cfg.KeyName))
	if err != nil {
		return fail(fmt.Errorf("set up tx client: %w", err))
	}

	l := logger.With().Str("component", "da_tx_signer").Str("address", addr.String()).Logger()
	if bal, err := user.QueryAccountBalance(ctx, conn, encCfg.InterfaceRegistry, addr, appconsts.BondDenom); err != nil {
		l.Warn().Err(err).Msg("could not query DA signer balance")
	} else {
		l.Info().Str("balance", bal.String()).Str("grpc", cfg.GRPCAddress).Msg("DA tx signer ready")
	}

	estimator := gasestimation.NewGasEstimatorClient(conn)
	return &Submitter{
		tc:     tc,
		conn:   conn,
		cancel: cancel,
		price: func(ctx context.Context, p gasestimation.TxPriority) (float64, error) {
			resp, err := estimator.EstimateGasPrice(ctx, &gasestimation.EstimateGasPriceRequest{TxPriority: p})
			if err != nil {
				return 0, err
			}
			return resp.EstimatedGasPrice, nil
		},
		account: cfg.KeyName,
	}, nil
}

func openKeyring(cfg Config, cdc codec.Codec) (keyring.Keyring, error) {
	if cfg.KeyringBackend == keyring.BackendTest {
		return keyring.New(app.Name, keyring.BackendTest, cfg.KeyringDir, nil, cdc)
	}
	// cosmos' keyring.New prompts on a TTY regardless of the reader it is
	// given, so open the file backend directly with the configured passphrase.
	db, err := keyring99.Open(keyring99.Config{
		AllowedBackends:  []keyring99.BackendType{keyring99.FileBackend},
		ServiceName:      app.Name,
		FileDir:          filepath.Join(cfg.KeyringDir, "keyring-file"),
		FilePasswordFunc: func(string) (string, error) { return cfg.KeyringPassphrase, nil },
	})
	if err != nil {
		return nil, err
	}
	return keyring.NewInMemoryWithKeyring(db, cdc), nil
}

func dial(cfg Config) (*grpc.ClientConn, error) {
	creds := insecure.NewCredentials()
	if cfg.GRPCTLS {
		creds = credentials.NewTLS(&tls.Config{MinVersion: tls.VersionTLS12})
	}
	opts := []grpc.DialOption{grpc.WithTransportCredentials(creds)}
	if token := cfg.GRPCAuthToken; token != "" {
		opts = append(opts, grpc.WithUnaryInterceptor(func(ctx context.Context, method string, req, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
			return invoker(metadata.AppendToOutgoingContext(ctx, "x-token", token), method, req, reply, cc, opts...)
		}))
	}
	return grpc.NewClient(cfg.GRPCAddress, opts...)
}

// Submit signs and broadcasts one PayForBlobs for blobs and returns the
// inclusion height.
func (s *Submitter) Submit(ctx context.Context, blobs []*blobrpc.Blob, opts *blobrpc.SubmitOptions) (uint64, error) {
	v4, err := toV4(blobs)
	if err != nil {
		return 0, err
	}
	if opts == nil {
		opts = &blobrpc.SubmitOptions{}
	}
	if opts.SignerAddress != "" {
		return 0, errSignerAddress
	}
	account := s.account
	if opts.KeyName != "" {
		account = opts.KeyName
	}
	acc := s.tc.Account(account)
	if acc == nil {
		return 0, fmt.Errorf("account %q not loaded; is it in the keyring and funded?", account)
	}

	txOpts, err := txOptions(ctx, acc.Address().String(), v4, opts, s.price)
	if err != nil {
		return 0, err
	}

	resp, err := s.tc.SubmitPayForBlobWithAccount(ctx, account, v4, txOpts...)
	if err != nil {
		return 0, err
	}
	return uint64(resp.Height), nil
}

// Close stops the tx client and releases the gRPC connection.
func (s *Submitter) Close() error {
	s.cancel()
	return s.conn.Close()
}

// toV4 converts ev-node's go-square v3 blobs to the v4 type celestia-app uses.
func toV4(blobs []*blobrpc.Blob) ([]*share.Blob, error) {
	out := make([]*share.Blob, len(blobs))
	for i, b := range blobs {
		ns, err := share.NewNamespaceFromBytes(b.Namespace().Bytes())
		if err != nil {
			return nil, fmt.Errorf("blob %d namespace: %w", i, err)
		}
		out[i], err = share.NewBlob(ns, b.Data(), b.ShareVersion(), b.Signer())
		if err != nil {
			return nil, fmt.Errorf("blob %d: %w", i, err)
		}
	}
	return out, nil
}

// txOptions maps SubmitOptions onto TxOptions. With no gas fields set it
// returns none, and the TxClient estimates gas and price itself.
func txOptions(ctx context.Context, signer string, blobs []*share.Blob, o *blobrpc.SubmitOptions, price priceEstimator) ([]user.TxOption, error) {
	var out []user.TxOption
	if o.FeeGranterAddress != "" {
		granter, err := sdk.AccAddressFromBech32(o.FeeGranterAddress)
		if err != nil {
			return nil, fmt.Errorf("fee granter address: %w", err)
		}
		out = append(out, user.SetFeeGranter(granter))
	}
	if !o.IsGasPriceSet && o.Gas == 0 && o.TxPriority == 0 && o.MaxGasPrice == 0 {
		return out, nil
	}

	gas := o.Gas
	if gas == 0 {
		msg, err := blobtypes.NewMsgPayForBlobs(signer, 0, blobs...)
		if err != nil {
			return nil, err
		}
		gas = blobtypes.DefaultEstimateGas(msg)
	}
	gasPrice := o.GasPrice
	if !o.IsGasPriceSet {
		var err error
		if gasPrice, err = price(ctx, gasestimation.TxPriority(o.TxPriority)); err != nil {
			return nil, fmt.Errorf("estimate gas price: %w", err)
		}
		if o.MaxGasPrice > 0 && gasPrice > o.MaxGasPrice {
			gasPrice = o.MaxGasPrice
		}
	}
	return append(out, user.SetGasLimitAndGasPrice(gas, gasPrice)), nil
}
