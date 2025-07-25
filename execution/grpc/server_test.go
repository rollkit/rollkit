package grpc

import (
	"context"
	"errors"
	"testing"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/timestamppb"

	pb "github.com/evstack/ev-node/types/pb/evnode/v1"
)

func TestServer_InitChain(t *testing.T) {
	ctx := context.Background()
	genesisTime := time.Now()
	initialHeight := uint64(1)
	chainID := "test-chain"
	expectedStateRoot := []byte("state_root")
	expectedMaxBytes := uint64(1000000)

	tests := []struct {
		name     string
		req      *pb.InitChainRequest
		mockFunc func(ctx context.Context, genesisTime time.Time, initialHeight uint64, chainID string) ([]byte, uint64, error)
		wantErr  bool
		wantCode connect.Code
	}{
		{
			name: "success",
			req: &pb.InitChainRequest{
				GenesisTime:   timestamppb.New(genesisTime),
				InitialHeight: initialHeight,
				ChainId:       chainID,
			},
			mockFunc: func(ctx context.Context, gt time.Time, ih uint64, cid string) ([]byte, uint64, error) {
				return expectedStateRoot, expectedMaxBytes, nil
			},
			wantErr: false,
		},
		{
			name: "missing genesis time",
			req: &pb.InitChainRequest{
				InitialHeight: initialHeight,
				ChainId:       chainID,
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "missing initial height",
			req: &pb.InitChainRequest{
				GenesisTime: timestamppb.New(genesisTime),
				ChainId:     chainID,
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "missing chain ID",
			req: &pb.InitChainRequest{
				GenesisTime:   timestamppb.New(genesisTime),
				InitialHeight: initialHeight,
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "executor error",
			req: &pb.InitChainRequest{
				GenesisTime:   timestamppb.New(genesisTime),
				InitialHeight: initialHeight,
				ChainId:       chainID,
			},
			mockFunc: func(ctx context.Context, gt time.Time, ih uint64, cid string) ([]byte, uint64, error) {
				return nil, 0, errors.New("init chain failed")
			},
			wantErr:  true,
			wantCode: connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockExecutor{
				initChainFunc: tt.mockFunc,
			}
			server := NewServer(mockExec)

			req := connect.NewRequest(tt.req)
			resp, err := server.InitChain(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					if connectErr.Code() != tt.wantCode {
						t.Errorf("expected error code %v, got %v", tt.wantCode, connectErr.Code())
					}
				} else {
					t.Errorf("expected connect error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(resp.Msg.StateRoot) != string(expectedStateRoot) {
				t.Errorf("expected state root %s, got %s", expectedStateRoot, resp.Msg.StateRoot)
			}
			if resp.Msg.MaxBytes != expectedMaxBytes {
				t.Errorf("expected max bytes %d, got %d", expectedMaxBytes, resp.Msg.MaxBytes)
			}
		})
	}
}

func TestServer_GetTxs(t *testing.T) {
	ctx := context.Background()
	expectedTxs := [][]byte{[]byte("tx1"), []byte("tx2")}

	tests := []struct {
		name     string
		mockFunc func(ctx context.Context) ([][]byte, error)
		wantErr  bool
		wantCode connect.Code
	}{
		{
			name: "success",
			mockFunc: func(ctx context.Context) ([][]byte, error) {
				return expectedTxs, nil
			},
			wantErr: false,
		},
		{
			name: "executor error",
			mockFunc: func(ctx context.Context) ([][]byte, error) {
				return nil, errors.New("get txs failed")
			},
			wantErr:  true,
			wantCode: connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockExecutor{
				getTxsFunc: tt.mockFunc,
			}
			server := NewServer(mockExec)

			req := connect.NewRequest(&pb.GetTxsRequest{})
			resp, err := server.GetTxs(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					if connectErr.Code() != tt.wantCode {
						t.Errorf("expected error code %v, got %v", tt.wantCode, connectErr.Code())
					}
				} else {
					t.Errorf("expected connect error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if len(resp.Msg.Txs) != len(expectedTxs) {
				t.Fatalf("expected %d txs, got %d", len(expectedTxs), len(resp.Msg.Txs))
			}
			for i, tx := range resp.Msg.Txs {
				if string(tx) != string(expectedTxs[i]) {
					t.Errorf("tx %d: expected %s, got %s", i, expectedTxs[i], tx)
				}
			}
		})
	}
}

func TestServer_ExecuteTxs(t *testing.T) {
	ctx := context.Background()
	txs := [][]byte{[]byte("tx1"), []byte("tx2")}
	blockHeight := uint64(10)
	timestamp := time.Now()
	prevStateRoot := []byte("prev_state_root")
	expectedStateRoot := []byte("new_state_root")
	expectedMaxBytes := uint64(2000000)

	tests := []struct {
		name     string
		req      *pb.ExecuteTxsRequest
		mockFunc func(ctx context.Context, txs [][]byte, blockHeight uint64, timestamp time.Time, prevStateRoot []byte) ([]byte, uint64, error)
		wantErr  bool
		wantCode connect.Code
	}{
		{
			name: "success",
			req: &pb.ExecuteTxsRequest{
				Txs:           txs,
				BlockHeight:   blockHeight,
				Timestamp:     timestamppb.New(timestamp),
				PrevStateRoot: prevStateRoot,
			},
			mockFunc: func(ctx context.Context, txs [][]byte, bh uint64, ts time.Time, psr []byte) ([]byte, uint64, error) {
				return expectedStateRoot, expectedMaxBytes, nil
			},
			wantErr: false,
		},
		{
			name: "missing block height",
			req: &pb.ExecuteTxsRequest{
				Txs:           txs,
				Timestamp:     timestamppb.New(timestamp),
				PrevStateRoot: prevStateRoot,
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "missing timestamp",
			req: &pb.ExecuteTxsRequest{
				Txs:           txs,
				BlockHeight:   blockHeight,
				PrevStateRoot: prevStateRoot,
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "missing prev state root",
			req: &pb.ExecuteTxsRequest{
				Txs:         txs,
				BlockHeight: blockHeight,
				Timestamp:   timestamppb.New(timestamp),
			},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "executor error",
			req: &pb.ExecuteTxsRequest{
				Txs:           txs,
				BlockHeight:   blockHeight,
				Timestamp:     timestamppb.New(timestamp),
				PrevStateRoot: prevStateRoot,
			},
			mockFunc: func(ctx context.Context, txs [][]byte, bh uint64, ts time.Time, psr []byte) ([]byte, uint64, error) {
				return nil, 0, errors.New("execute txs failed")
			},
			wantErr:  true,
			wantCode: connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockExecutor{
				executeTxsFunc: tt.mockFunc,
			}
			server := NewServer(mockExec)

			req := connect.NewRequest(tt.req)
			resp, err := server.ExecuteTxs(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					if connectErr.Code() != tt.wantCode {
						t.Errorf("expected error code %v, got %v", tt.wantCode, connectErr.Code())
					}
				} else {
					t.Errorf("expected connect error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}

			if string(resp.Msg.UpdatedStateRoot) != string(expectedStateRoot) {
				t.Errorf("expected state root %s, got %s", expectedStateRoot, resp.Msg.UpdatedStateRoot)
			}
			if resp.Msg.MaxBytes != expectedMaxBytes {
				t.Errorf("expected max bytes %d, got %d", expectedMaxBytes, resp.Msg.MaxBytes)
			}
		})
	}
}

func TestServer_SetFinal(t *testing.T) {
	ctx := context.Background()
	blockHeight := uint64(100)

	tests := []struct {
		name     string
		req      *pb.SetFinalRequest
		mockFunc func(ctx context.Context, blockHeight uint64) error
		wantErr  bool
		wantCode connect.Code
	}{
		{
			name: "success",
			req: &pb.SetFinalRequest{
				BlockHeight: blockHeight,
			},
			mockFunc: func(ctx context.Context, bh uint64) error {
				return nil
			},
			wantErr: false,
		},
		{
			name:     "missing block height",
			req:      &pb.SetFinalRequest{},
			wantErr:  true,
			wantCode: connect.CodeInvalidArgument,
		},
		{
			name: "executor error",
			req: &pb.SetFinalRequest{
				BlockHeight: blockHeight,
			},
			mockFunc: func(ctx context.Context, bh uint64) error {
				return errors.New("set final failed")
			},
			wantErr:  true,
			wantCode: connect.CodeInternal,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockExec := &mockExecutor{
				setFinalFunc: tt.mockFunc,
			}
			server := NewServer(mockExec)

			req := connect.NewRequest(tt.req)
			_, err := server.SetFinal(ctx, req)

			if tt.wantErr {
				if err == nil {
					t.Fatalf("expected error but got none")
				}
				var connectErr *connect.Error
				if errors.As(err, &connectErr) {
					if connectErr.Code() != tt.wantCode {
						t.Errorf("expected error code %v, got %v", tt.wantCode, connectErr.Code())
					}
				} else {
					t.Errorf("expected connect error, got %v", err)
				}
				return
			}

			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
		})
	}
}
