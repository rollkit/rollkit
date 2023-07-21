package v1

import (
	"bytes"
	"fmt"
	"math/rand"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/cometbft/cometbft/abci/example/code"
	"github.com/cometbft/cometbft/abci/example/kvstore"
	abci "github.com/cometbft/cometbft/abci/types"
	"github.com/cometbft/cometbft/config"
	"github.com/cometbft/cometbft/libs/log"
	"github.com/cometbft/cometbft/proxy"
	"github.com/cometbft/cometbft/types"
	"github.com/stretchr/testify/require"

	"github.com/rollkit/rollkit/mempool"
)

// application extends the KV store application by overriding CheckTx to provide
// transaction priority based on the value in the key/value pair.
type application struct {
	*kvstore.Application
}

type testTx struct {
	tx       types.Tx
	priority int64
}

func (app *application) CheckTx(req abci.RequestCheckTx) abci.ResponseCheckTx {
	var (
		priority int64
		sender   string
	)

	// infer the priority from the raw transaction value (sender=key=value)
	parts := bytes.Split(req.Tx, []byte("="))
	if len(parts) == 3 {
		v, err := strconv.ParseInt(string(parts[2]), 10, 64)
		if err != nil {
			return abci.ResponseCheckTx{
				Priority:  priority,
				Code:      100,
				GasWanted: 1,
			}
		}

		priority = v
		sender = string(parts[0])
	} else {
		return abci.ResponseCheckTx{
			Priority:  priority,
			Code:      101,
			GasWanted: 1,
		}
	}

	return abci.ResponseCheckTx{
		Priority:  priority,
		Sender:    sender,
		Code:      code.CodeTypeOK,
		GasWanted: 1,
	}
}

func setup(t testing.TB, cacheSize int, options ...TxMempoolOption) *TxMempool {
	t.Helper()

	app := &application{kvstore.NewApplication()}
	cc := proxy.NewLocalClientCreator(app)

	cfg := config.ResetTestRoot(strings.ReplaceAll(t.Name(), "/", "|"))
	cfg.Mempool.CacheSize = cacheSize

	appConnMem, err := cc.NewABCIClient()
	require.NoError(t, err)
	require.NoError(t, appConnMem.Start())

	t.Cleanup(func() {
		os.RemoveAll(cfg.RootDir)
		require.NoError(t, appConnMem.Stop())
	})

	return NewTxMempool(log.TestingLogger().With("test", t.Name()), cfg.Mempool, appConnMem, 0, options...)
}

// mustCheckTx invokes txmp.CheckTx for the given transaction and waits until
// its callback has finished executing. It fails t if CheckTx fails.
func mustCheckTx(t *testing.T, txmp *TxMempool, spec string) {
	done := make(chan struct{})
	if err := txmp.CheckTx([]byte(spec), func(*abci.Response) {
		close(done)
	}, mempool.TxInfo{}); err != nil {
		t.Fatalf("CheckTx for %q failed: %v", spec, err)
	}
	<-done
}

func checkTxs(t *testing.T, txmp *TxMempool, numTxs int, peerID uint16) []testTx {
	txs := make([]testTx, numTxs)
	txInfo := mempool.TxInfo{SenderID: peerID}

	rng := rand.New(rand.NewSource(time.Now().UnixNano())) //nolint:gosec

	for i := 0; i < numTxs; i++ {
		prefix := make([]byte, 20)
		_, err := rng.Read(prefix)
		require.NoError(t, err)

		priority := int64(rng.Intn(9999-1000) + 1000)

		txs[i] = testTx{
			tx:       []byte(fmt.Sprintf("sender-%d-%d=%X=%d", i, peerID, prefix, priority)),
			priority: priority,
		}
		require.NoError(t, txmp.CheckTx(txs[i].tx, nil, txInfo))
	}

	return txs
}
