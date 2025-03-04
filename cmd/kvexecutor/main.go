package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"google.golang.org/grpc"

	pb "github.com/rollkit/go-execution/types/pb/execution"
	"github.com/rollkit/rollkit/core/execution"
)

// executorServer wraps a KVExecutor to expose the Executor API over gRPC.
type executorServer struct {
	exec *execution.KVExecutor
}

func (s *executorServer) InitChain(ctx context.Context, req *pb.InitChainRequest) (*pb.InitChainResponse, error) {
	stateRoot, maxBytes, err := s.exec.InitChain(ctx, time.Unix(req.GenesisTime, 0), req.InitialHeight, req.ChainId)
	if err != nil {
		return nil, err
	}
	return &pb.InitChainResponse{
		StateRoot: stateRoot,
		MaxBytes:  maxBytes,
	}, nil
}

func (s *executorServer) GetTxs(ctx context.Context, req *pb.GetTxsRequest) (*pb.GetTxsResponse, error) {
	txs, err := s.exec.GetTxs(ctx)
	if err != nil {
		return nil, err
	}
	return &pb.GetTxsResponse{Txs: txs}, nil
}

func (s *executorServer) ExecuteTxs(ctx context.Context, req *pb.ExecuteTxsRequest) (*pb.ExecuteTxsResponse, error) {
	updatedStateRoot, maxBytes, err := s.exec.ExecuteTxs(ctx, req.Txs, req.BlockHeight, time.Unix(req.Timestamp, 0), req.PrevStateRoot)
	if err != nil {
		return nil, err
	}
	return &pb.ExecuteTxsResponse{
		UpdatedStateRoot: updatedStateRoot,
		MaxBytes:         maxBytes,
	}, nil
}

func (s *executorServer) SetFinal(ctx context.Context, req *pb.SetFinalRequest) (*pb.SetFinalResponse, error) {
	if err := s.exec.SetFinal(ctx, req.BlockHeight); err != nil {
		return nil, err
	}
	return &pb.SetFinalResponse{}, nil
}

func main() {
	httpAddr := flag.String("http", ":8080", "HTTP server address")
	grpcAddr := flag.String("executor_address", ":40041", "gRPC executor address (can be set via nodeConfig.ExecutorAddress)")
	flag.Parse()

	// Create a KVExecutor instance
	kvexec := execution.NewKVExecutor()

	// Start HTTP server for key-value operations
	httpMux := http.NewServeMux()
	httpMux.HandleFunc("/set", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "Only POST allowed", http.StatusMethodNotAllowed)
			return
		}
		key := r.FormValue("key")
		value := r.FormValue("value")
		if key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		kvexec.SetStoreValue(key, value)
		fmt.Fprintf(w, "Set %s = %s\n", key, value)
	})
	httpMux.HandleFunc("/get", func(w http.ResponseWriter, r *http.Request) {
		key := r.URL.Query().Get("key")
		if key == "" {
			http.Error(w, "missing key", http.StatusBadRequest)
			return
		}
		value, ok := kvexec.GetStoreValue(key)
		if !ok {
			http.Error(w, "key not found", http.StatusNotFound)
			return
		}
		fmt.Fprintf(w, "%s\n", value)
	})

	httpServer := &http.Server{
		Addr:    *httpAddr,
		Handler: httpMux,
	}
	go func() {
		log.Printf("Starting HTTP server on %s", *httpAddr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("HTTP server error: %v", err)
		}
	}()

	// Start gRPC server for the Executor API
	grpcLis, err := net.Listen("tcp", *grpcAddr)
	if err != nil {
		log.Fatalf("failed to listen on %s: %v", *grpcAddr, err)
	}
	grpcServer := grpc.NewServer()
	// Register our executorServer with the gRPC server
	srv := &executorServer{exec: kvexec}
	pb.RegisterExecutionServiceServer(grpcServer, srv)
	go func() {
		log.Printf("Starting gRPC executor server on %s", *grpcAddr)
		if err := grpcServer.Serve(grpcLis); err != nil {
			log.Fatalf("gRPC server error: %v", err)
		}
	}()

	// Listen for SIGINT or SIGTERM signals to gracefully shut down
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh
	log.Println("Shutting down servers...")

	// Shutdown HTTP server
	ctxShutdown, cancelShutdown := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelShutdown()
	if err := httpServer.Shutdown(ctxShutdown); err != nil {
		log.Printf("HTTP shutdown error: %v", err)
	}
	grpcServer.GracefulStop()
	log.Println("Servers stopped gracefully")
}
