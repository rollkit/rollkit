package rpc

import (
	"context"
	"crypto/rand"
	"fmt"
	"io"
	"net/http"

	//"fmt"
	//"io"
	//"net/http"
	//"net/url"

	"testing"
	"time"

	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/rollkit/rollkit/config"
	"github.com/rollkit/rollkit/mocks"
	"github.com/rollkit/rollkit/node"
	"github.com/rollkit/rollkit/types"

	//"github.com/rollkit/rollkit/rpc"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/mock"
	"github.com/stretchr/testify/require"
	abcicli "github.com/tendermint/tendermint/abci/client"
	abci "github.com/tendermint/tendermint/abci/types"
	tmconf "github.com/tendermint/tendermint/config"

	"github.com/tendermint/tendermint/libs/log"
	tmtypes "github.com/tendermint/tendermint/types"
)

type receiveDirectTxArgs struct {
	Tx []byte `json:"tx"`
}

func TestSequencerServer(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	app := &mocks.Application{}
	app.On("InitChain", mock.Anything).Return(abci.ResponseInitChain{})
	app.On("CheckTx", mock.Anything).Return(abci.ResponseCheckTx{})
	app.On("BeginBlock", mock.Anything).Return(abci.ResponseBeginBlock{})
	app.On("DeliverTx", mock.Anything).Return(abci.ResponseDeliverTx{})
	app.On("EndBlock", mock.Anything).Return(abci.ResponseEndBlock{})
	app.On("Commit", mock.Anything).Return(abci.ResponseCommit{})
	app.On("GetAppHash", mock.Anything).Return(abci.ResponseGetAppHash{})

	key, _, _ := crypto.GenerateEd25519Key(rand.Reader)
	signingKey, _, _ := crypto.GenerateEd25519Key(rand.Reader)

	blockManagerConfig := config.BlockManagerConfig{
		BlockTime:   1 * time.Second,
		NamespaceID: types.NamespaceID{1, 2, 3, 4, 5, 6, 7, 8},
	}

	node, err := node.NewNode(context.Background(), config.NodeConfig{
		DALayer:              "mock",
		Aggregator:           true,
		BlockManagerConfig:   blockManagerConfig,
		ProgressiveSequencer: true,
	}, key, signingKey, abcicli.NewLocalClient(nil, app), &tmtypes.GenesisDoc{ChainID: "test"}, log.TestingLogger())
	assert.False(node.IsRunning())
	assert.NoError(err)
	err = node.Start()
	assert.NoError(err)
	defer func() {
		err := node.Stop()
		assert.NoError(err)
	}()
	assert.True(node.IsRunning())

	require.NoError(err)

	conf := tmconf.DefaultRPCConfig()
	server := NewServer(
		node,
		conf,
		log.TestingLogger(),
		node.ReceiveDirectTx(),
	)
	err = server.Start()
	assert.NoError(err)

	fmt.Println("Letting server spin up...")
	time.Sleep(3 * time.Second)
	fmt.Println("Ok.")

	/*arg := receiveDirectTxArgs{
		Tx: []byte(fmt.Sprintf("abc%d", 1)),
	}*/

	/*jsonReq, err := json2.EncodeClientRequest("receive_direct_tx", &arg)
	assert.NoError(err)
	fmt.Println("sending: ", string(jsonReq))*/
	resp, err := http.Get("http://127.0.0.1:26657/receive_direct_tx?tx=0101")
	assert.NoError(err)
	b, err := io.ReadAll(resp.Body)
	fmt.Println("got: ", string(b))
	assert.NoError(err)
	/*sendAsync := func(i int, out chan *http.Response) {
		arg := receiveDirectTxArgs{
			Tx: []byte(fmt.Sprintf("abc%d", i)),
		}
		encoded, err := json.Marshal(arg)
		assert.NoError(err)
		requestBuf := bytes.NewBuffer(encoded)
		fmt.Println("Sending")
		resp, err := http.Post("http://127.0.0.1:26657/receive_direct_tx", "application/json", requestBuf)
		fmt.Println("Sent")
		assert.NoError(err)
		out <- resp
	}

	resps := make(chan *http.Response)
	sendAsync(0, resps)
	resp1 := <-resps
	fmt.Println("Got:")
	fmt.Println(resp1)*/
	//sendAsync := func(i int, out chan *http.Response) {
	/*resp, err := http.PostForm("http://127.0.0.1:26657/receive_direct_tx",
		url.Values{
			"tx": []string{fmt.Sprintf("abc%d", i)},
		},
	)
	out <- resp
	assert.NoError(err)*/

	//}

	/*resps := make(chan *http.Response)
	for i := 0; i < 5; i++ {
		go sendAsync(i, resps)
	}
	time.Sleep(3 * time.Second)
	for i := 5; i < 10; i++ {
		go sendAsync(i, resps)
	}
	time.Sleep(3 * time.Second)
	for i := 10; i < 15; i++ {
		go sendAsync(i, resps)
	}
	time.Sleep(3 * time.Second)
	for i := 15; i < 20; i++ {
		go sendAsync(i, resps)
	}
	time.Sleep(3 * time.Second)
	// "Genesis" block: should be height = 1
	for i := 0; i < 10; i++ {
		r := <-resps
		b, err := io.ReadAll(r.Body)
		assert.NoError(err)
		assert.Equal(string(b), `{"jsonrpc":"2.0","result":"includd in block 1","id":-1}
	`)
	}
	// Next block height = 2
	for i := 10; i < 15; i++ {
		r := <-resps
		b, err := io.ReadAll(r.Body)
		assert.NoError(err)
		assert.Equal(string(b), `{"jsonrpc":"2.0","result":"includd in block 2","id":-1}
	`)
	}
	// Next block height = 3
	for i := 15; i < 20; i++ {
		r := <-resps
		b, err := io.ReadAll(r.Body)
		assert.NoError(err)
		assert.Equal(string(b), `{"jsonrpc":"2.0","result":"includd in block 3","id":-1}
	`)
	}*/

	/*conf := tmconf.DefaultRPCConfig()
	server := rpc.NewServer(node, conf, node.Logger, node.ReceiveDirectTx)
	err = server.Start()
	assert.NoError(err)*/

	/*conf := tmconf.DefaultRPCConfig()
		ss := NewSequencerServer(node, conf, node.Logger)
		err = ss.Start()
		assert.NoError(err)

		sendAsync := func(i int, out chan *http.Response) {
			resp, err := http.PostForm("http://127.0.0.1:2007/tx",
				url.Values{
					"tx": []string{fmt.Sprintf("abc%d", i)},
				},
			)
			out <- resp
			assert.NoError(err)
		}

		resps := make(chan *http.Response)
		for i := 0; i < 5; i++ {
			go sendAsync(i, resps)
		}
		time.Sleep(3 * time.Second)
		for i := 5; i < 10; i++ {
			go sendAsync(i, resps)
		}
		time.Sleep(3 * time.Second)
		for i := 10; i < 15; i++ {
			go sendAsync(i, resps)
		}
		time.Sleep(3 * time.Second)
		for i := 15; i < 20; i++ {
			go sendAsync(i, resps)
		}
		time.Sleep(3 * time.Second)
		// "Genesis" block: should be height = 1
		for i := 0; i < 10; i++ {
			r := <-resps
			b, err := io.ReadAll(r.Body)
			assert.NoError(err)
			assert.Equal(string(b), `{"jsonrpc":"2.0","result":"includd in block 1","id":-1}
	`)
		}
		// Next block height = 2
		for i := 10; i < 15; i++ {
			r := <-resps
			b, err := io.ReadAll(r.Body)
			assert.NoError(err)
			assert.Equal(string(b), `{"jsonrpc":"2.0","result":"includd in block 2","id":-1}
	`)
		}
		// Next block height = 3
		for i := 15; i < 20; i++ {
			r := <-resps
			b, err := io.ReadAll(r.Body)
			assert.NoError(err)
			assert.Equal(string(b), `{"jsonrpc":"2.0","result":"includd in block 3","id":-1}
	`)
		}
	*/
}
