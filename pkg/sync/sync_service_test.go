package sync

import (
	"context"
	cryptoRand "crypto/rand"
	"math/rand"
	"path/filepath"
	"testing"
	"time"

	sdklog "cosmossdk.io/log"
	"github.com/ipfs/go-datastore"
	"github.com/ipfs/go-datastore/sync"
	"github.com/libp2p/go-libp2p/core/crypto"
	mocknet "github.com/libp2p/go-libp2p/p2p/net/mock"
	"github.com/rollkit/rollkit/pkg/config"
	genesispkg "github.com/rollkit/rollkit/pkg/genesis"
	"github.com/rollkit/rollkit/pkg/p2p"
	"github.com/rollkit/rollkit/pkg/p2p/key"
	"github.com/rollkit/rollkit/pkg/signer"
	"github.com/rollkit/rollkit/pkg/signer/noop"
	"github.com/rollkit/rollkit/types"
	"github.com/stretchr/testify/require"
)

func TestHeaderSyncServiceRestart(t *testing.T) {
	mainKV := sync.MutexWrap(datastore.NewMapDatastore())
	pk, _, err := crypto.GenerateEd25519Key(cryptoRand.Reader)
	require.NoError(t, err)
	noopSigner, err := noop.NewNoopSigner(pk)
	require.NoError(t, err)
	rnd := rand.New(rand.NewSource(1)) // nolint:gosec // test code only
	mn := mocknet.New()

	proposerAddr := []byte("test")
	genesisDoc := genesispkg.Genesis{
		ChainID:            "test-chain-id",
		GenesisDAStartTime: time.Now(),
		InitialHeight:      1,
		ProposerAddress:    proposerAddr,
	}
	conf := config.DefaultConfig
	conf.RootDir = t.TempDir()
	nodeKey, err := key.LoadOrGenNodeKey(filepath.Dir(conf.ConfigPath()))
	require.NoError(t, err)
	logger := sdklog.NewTestLogger(t)
	priv := nodeKey.PrivKey
	h, err := mn.AddPeer(priv, nil)
	require.NoError(t, err)

	p2pClient, err := p2p.NewClientWithHost(conf, nodeKey, mainKV, logger, p2p.NopMetrics(), h)
	require.NoError(t, err)

	// Start p2p client before creating sync service
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	require.NoError(t, p2pClient.Start(ctx))

	svc, err := NewHeaderSyncService(mainKV, conf, genesisDoc, p2pClient, logger)
	require.NoError(t, err)
	err = svc.Start(ctx)
	require.NoError(t, err)

	// broadcast genesis block
	headerConfig := types.HeaderConfig{
		Height:   genesisDoc.InitialHeight,
		DataHash: bytesN(rnd, 32),
		AppHash:  bytesN(rnd, 32),
		Signer:   noopSigner,
	}
	signedHeader, err := types.GetRandomSignedHeaderCustom(&headerConfig, genesisDoc.ChainID)
	require.NoError(t, err)
	require.NoError(t, signedHeader.Validate())
	require.NoError(t, svc.WriteToStoreAndBroadcast(ctx, signedHeader))

	for i := genesisDoc.InitialHeight + 1; i < 2; i++ {
		signedHeader = nextHeader(t, signedHeader, genesisDoc.ChainID, noopSigner)
		t.Logf("signed header: %d", i)
		require.NoError(t, svc.WriteToStoreAndBroadcast(ctx, signedHeader))
	}

	// then stop and restart service
	_ = p2pClient.Close()
	_ = svc.Stop(ctx)
	cancel()

	h2, err := mn.AddPeer(priv, nil)
	require.NoError(t, err)
	p2pClient, err = p2p.NewClientWithHost(conf, nodeKey, mainKV, logger, p2p.NopMetrics(), h2)
	require.NoError(t, err)

	// Start p2p client again
	ctx, cancel = context.WithCancel(t.Context())
	defer cancel()
	err = p2pClient.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = p2pClient.Close() })

	svc, err = NewHeaderSyncService(mainKV, conf, genesisDoc, p2pClient, logger)
	require.NoError(t, err)
	err = svc.Start(ctx)
	require.NoError(t, err)
	t.Cleanup(func() { _ = svc.Stop(context.Background()) })
	// done with stop and restart service

	// broadcast another 2 example blocks
	for i := signedHeader.Height() + 1; i < 2; i++ {
		signedHeader = nextHeader(t, signedHeader, genesisDoc.ChainID, noopSigner)
		t.Logf("signed header: %d", i)
		require.NoError(t, svc.WriteToStoreAndBroadcast(ctx, signedHeader))
	}
	cancel()
}

func nextHeader(t *testing.T, previousHeader *types.SignedHeader, chainID string, noopSigner signer.Signer) *types.SignedHeader {
	newSignedHeader := &types.SignedHeader{
		Header: types.GetRandomNextHeader(previousHeader.Header, chainID),
		Signer: previousHeader.Signer,
	}
	b, err := newSignedHeader.Header.MarshalBinary()
	require.NoError(t, err)
	signature, err := noopSigner.Sign(b)
	require.NoError(t, err)
	newSignedHeader.Signature = signature
	require.NoError(t, newSignedHeader.Validate())
	previousHeader = newSignedHeader
	return previousHeader
}

func bytesN(r *rand.Rand, n int) []byte {
	data := make([]byte, n)
	_, _ = r.Read(data)
	return data
}
