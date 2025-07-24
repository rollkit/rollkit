package cmd

import (
	"bytes"
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/multiformats/go-multiaddr"
	"github.com/spf13/cobra"
	"github.com/spf13/viper"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/evstack/ev-node/pkg/config"
	"github.com/evstack/ev-node/pkg/p2p"
	"github.com/evstack/ev-node/pkg/rpc/server"
	testmocks "github.com/evstack/ev-node/test/mocks"
	"github.com/evstack/ev-node/types/pb/rollkit/v1/v1connect"
)

type contextKey string

const viperKey contextKey = "viper"

// executeCommandC executes the command and captures its output
func executeCommandC(root *cobra.Command, args ...string) (string, error) {
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)

	err := root.Execute()
	return strings.TrimSpace(buf.String()), err
}

func TestNetInfoCmd_Success(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockP2P := new(testmocks.MockP2PRPC)

	mockNodeID := "12D3KooWExampleNodeID1234567890"
	mockListenAddr1 := "/ip4/127.0.0.1/tcp/7676"
	mockListenAddr2 := "/ip6/::1/tcp/7677"
	mockPeerID1Str := "12D3KooWJHLDoXhmgYe6FEbujPzMQJvJ9JyGwRR2VjRM4f7Udvte"
	mockPeerAddr1Str := "/ip4/192.168.1.100/tcp/7676"
	mockPeerID2Str := "12D3KooWJHLDoXhmgYe6FEbujPzMQJvJ9JyGwRR2VjRM4f7Udvte"
	mockPeerAddr2Str := "/ip4/192.168.1.101/tcp/7676"

	mockNetInfo := p2p.NetworkInfo{
		ID:            mockNodeID,
		ListenAddress: []string{mockListenAddr1, mockListenAddr2},
	}

	peerID1, err := peer.Decode(mockPeerID1Str)
	require.NoError(err)
	peerMultiaddr1, err := multiaddr.NewMultiaddr(mockPeerAddr1Str)
	require.NoError(err)
	addrInfo1 := peer.AddrInfo{ID: peerID1, Addrs: []multiaddr.Multiaddr{peerMultiaddr1}}

	peerID2, err := peer.Decode(mockPeerID2Str)
	require.NoError(err)
	peerMultiaddr2, err := multiaddr.NewMultiaddr(mockPeerAddr2Str)
	require.NoError(err)
	addrInfo2 := peer.AddrInfo{ID: peerID2, Addrs: []multiaddr.Multiaddr{peerMultiaddr2}}

	mockPeers := []peer.AddrInfo{addrInfo1, addrInfo2}

	mockP2P.On("GetNetworkInfo").Return(mockNetInfo, nil)
	mockP2P.On("GetPeers").Return(mockPeers, nil)

	p2pServer := server.NewP2PServer(mockP2P)
	mux := http.NewServeMux()

	p2pPath, p2pHandler := v1connect.NewP2PServiceHandler(p2pServer)
	mux.Handle(p2pPath, p2pHandler)

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	tempDir, err := os.MkdirTemp("", "rollkit-test-home-*")
	require.NoError(err)
	defer os.RemoveAll(tempDir)

	v := viper.New()
	rpcAddr := strings.TrimPrefix(httpServer.URL, "http://")
	v.Set(config.FlagRPCAddress, rpcAddr)
	v.Set(config.FlagRootDir, tempDir)

	rootCmd := &cobra.Command{Use: "root"}
	rootCmd.PersistentFlags().String(config.FlagRootDir, tempDir, "Root directory for config and data")
	rootCmd.PersistentFlags().String(config.FlagRPCAddress, rpcAddr, "RPC listen address")

	err = v.BindPFlag(config.FlagRootDir, rootCmd.PersistentFlags().Lookup(config.FlagRootDir))
	require.NoError(err)
	err = v.BindPFlag(config.FlagRPCAddress, rootCmd.PersistentFlags().Lookup(config.FlagRPCAddress))
	require.NoError(err)

	NetInfoCmd.SetContext(context.WithValue(context.Background(), viperKey, v))
	rootCmd.AddCommand(NetInfoCmd)

	output, err := executeCommandC(rootCmd, "net-info", "--rollkit.rpc.address="+rpcAddr)

	require.NoError(err, "Command execution failed: %s", output)
	t.Log("Command Output:\n", output)

	assert.Contains(output, "NODE INFORMATION")
	assert.Contains(output, fmt.Sprintf("Node ID:      \033[1;36m%s\033[0m", mockNodeID))
	assert.Contains(output, "Listen Addrs:")
	assert.Contains(output, fmt.Sprintf("Addr: \033[1;36m%s\033[0m", mockListenAddr1))
	assert.Contains(output, fmt.Sprintf("Full: \033[1;32m%s/p2p/%s\033[0m", mockListenAddr1, mockNodeID))
	assert.Contains(output, fmt.Sprintf("Addr: \033[1;36m%s\033[0m", mockListenAddr2))
	assert.Contains(output, fmt.Sprintf("Full: \033[1;32m%s/p2p/%s\033[0m", mockListenAddr2, mockNodeID))

	assert.Contains(output, "CONNECTED PEERS: \033[1;33m2\033[0m")
	assert.Contains(output, "PEER ID")
	assert.Contains(output, "ADDRESS")

	truncatedPeerID1 := mockPeerID1Str[:15] + "..."
	expectedPeerAddrOutput1 := addrInfo1.String()
	assert.Contains(output, fmt.Sprintf("%-5d \033[1;34m%-20s\033[0m %s", 1, truncatedPeerID1, expectedPeerAddrOutput1), "Peer 1 details mismatch")

	truncatedPeerID2 := mockPeerID2Str[:15] + "..."
	expectedPeerAddrOutput2 := addrInfo2.String()
	assert.Contains(output, fmt.Sprintf("%-5d \033[1;34m%-20s\033[0m %s", 2, truncatedPeerID2, expectedPeerAddrOutput2), "Peer 2 details mismatch")

	mockP2P.AssertExpectations(t)
}

func TestNetInfoCmd_NoPeers(t *testing.T) {
	assert := assert.New(t)
	require := require.New(t)

	mockP2P := new(testmocks.MockP2PRPC)

	mockNodeID := "12D3KooWExampleNodeID1234567890"
	mockListenAddr1 := "/ip4/127.0.0.1/tcp/7676"
	mockListenAddr2 := "/ip6/::1/tcp/7677"

	mockNetInfo := p2p.NetworkInfo{
		ID:            mockNodeID,
		ListenAddress: []string{mockListenAddr1, mockListenAddr2},
	}

	mockPeers := []peer.AddrInfo{}

	mockP2P.On("GetNetworkInfo").Return(mockNetInfo, nil)
	mockP2P.On("GetPeers").Return(mockPeers, nil)

	p2pServer := server.NewP2PServer(mockP2P)
	mux := http.NewServeMux()
	p2pPath, p2pHandler := v1connect.NewP2PServiceHandler(p2pServer)
	mux.Handle(p2pPath, p2pHandler)

	httpServer := httptest.NewServer(mux)
	defer httpServer.Close()

	tempDir, err := os.MkdirTemp("", "rollkit-test-home-nopeer-*")
	require.NoError(err)
	defer os.RemoveAll(tempDir)

	// Configure Viper to use the test server's address and temp home
	v := viper.New()
	rpcAddr := strings.TrimPrefix(httpServer.URL, "http://")
	v.Set(config.FlagRPCAddress, rpcAddr)
	v.Set(config.FlagRootDir, tempDir)

	rootCmd := &cobra.Command{Use: "root"}
	rootCmd.PersistentFlags().String(config.FlagRootDir, tempDir, "Root directory for config and data")
	rootCmd.PersistentFlags().String(config.FlagRPCAddress, rpcAddr, "RPC listen address")

	err = v.BindPFlag(config.FlagRootDir, rootCmd.PersistentFlags().Lookup(config.FlagRootDir))
	require.NoError(err)
	err = v.BindPFlag(config.FlagRPCAddress, rootCmd.PersistentFlags().Lookup(config.FlagRPCAddress))
	require.NoError(err)

	NetInfoCmd.SetContext(context.WithValue(context.Background(), viperKey, v))
	rootCmd.AddCommand(NetInfoCmd)

	output, err := executeCommandC(rootCmd, "net-info", "--rollkit.rpc.address="+rpcAddr)

	require.NoError(err, "Command execution failed: %s", output)
	t.Log("Command Output:\n", output)

	assert.Contains(output, "NODE INFORMATION")
	assert.Contains(output, fmt.Sprintf("Node ID:      \033[1;36m%s\033[0m", mockNodeID))
	assert.Contains(output, "Listen Addrs:")
	assert.Contains(output, fmt.Sprintf("Addr: \033[1;36m%s\033[0m", mockListenAddr1))
	assert.Contains(output, fmt.Sprintf("Full: \033[1;32m%s/p2p/%s\033[0m", mockListenAddr1, mockNodeID))
	assert.Contains(output, fmt.Sprintf("Addr: \033[1;36m%s\033[0m", mockListenAddr2))
	assert.Contains(output, fmt.Sprintf("Full: \033[1;32m%s/p2p/%s\033[0m", mockListenAddr2, mockNodeID))

	assert.Contains(output, "CONNECTED PEERS: \033[1;33m0\033[0m")
	assert.Contains(output, "No peers connected")

	mockP2P.AssertExpectations(t)
}
