package test

import (
	"fmt"
	"math/rand"
	"net"
	"os"
	"strconv"
	"time"

	tmlog "github.com/tendermint/tendermint/libs/log"
	"google.golang.org/grpc"

	cmock "github.com/rollkit/rollkit/da/celestia/mock"
	grpcda "github.com/rollkit/rollkit/da/grpc"
	"github.com/rollkit/rollkit/da/grpc/mockserv"
	"github.com/rollkit/rollkit/store"
	"github.com/rollkit/rollkit/types"
)

const mockDaBlockTime = 100 * time.Millisecond

func getRandomTx() types.Tx {
	size := rand.Int()%100 + 100 //nolint:gosec
	return types.Tx(getRandomBytes(size))
}

func getRandomBytes(n int) []byte {
	data := make([]byte, n)
	_, _ = rand.Read(data) //nolint:gosec
	return data
}

// copy-pasted from store/store_test.go
func getRandomBlock(height uint64, nTxs int) *types.Block {
	block := &types.Block{
		SignedHeader: types.SignedHeader{
			Header: types.Header{
				BaseHeader: types.BaseHeader{
					Height: height,
				},
				AggregatorsHash: make([]byte, 32),
			}},
		Data: types.Data{
			Txs: make(types.Txs, nTxs),
			IntermediateStateRoots: types.IntermediateStateRoots{
				RawRootsList: make([][]byte, nTxs),
			},
		},
	}
	block.SignedHeader.Header.AppHash = getRandomBytes(32)

	for i := 0; i < nTxs; i++ {
		block.Data.Txs[i] = getRandomTx()
		block.Data.IntermediateStateRoots.RawRootsList[i] = getRandomBytes(32)
	}

	// TODO(tzdybal): see https://github.com/rollkit/rollkit/issues/143
	if nTxs == 0 {
		block.Data.Txs = nil
		block.Data.IntermediateStateRoots.RawRootsList = nil
	}

	return block
}

func startMockGRPCServ() *grpc.Server {
	conf := grpcda.DefaultConfig
	logger := tmlog.NewTMLogger(os.Stdout)

	kvStore, _ := store.NewDefaultInMemoryKVStore()
	srv := mockserv.GetServer(kvStore, conf, []byte(mockDaBlockTime.String()), logger)
	lis, err := net.Listen("tcp", conf.Host+":"+strconv.Itoa(conf.Port))
	if err != nil {
		fmt.Println(err)
		return nil
	}
	go func() {
		_ = srv.Serve(lis)
	}()
	return srv
}

func startMockCelestiaNodeServer() *cmock.Server {
	httpSrv := cmock.NewServer(mockDaBlockTime, tmlog.NewTMLogger(os.Stdout))
	l, err := net.Listen("tcp4", "127.0.0.1:26658")
	if err != nil {
		fmt.Println("failed to create listener for mock celestia-node RPC server, error: %w", err)
		return nil
	}
	err = httpSrv.Start(l)
	if err != nil {
		fmt.Println("can't start mock celestia-node RPC server")
		return nil
	}
	return httpSrv
}
