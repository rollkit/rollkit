package main

import (
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"

	"github.com/rollkit/go-sequencing/proxy/grpc"
	"github.com/rollkit/go-sequencing/test"
)

const (
	// MockSequencerAddress is the mock address for the gRPC server
	MockSequencerAddress = "localhost:50051"
)

func main() {
	lis, err := net.Listen("tcp", MockSequencerAddress)
	if err != nil {
		log.Fatalf("Failed to listen: %v", err)
	}

	dummySequencer := test.NewMultiRollupSequencer()

	srv := grpc.NewServer(dummySequencer, dummySequencer, dummySequencer)
	log.Printf("Listening on: %s:%s", "localhost", "50051")
	if err := srv.Serve(lis); err != nil {
		log.Fatal("error while serving:", err)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGINT)
	<-interrupt
	fmt.Println("\nCtrl+C pressed. Exiting...")
	os.Exit(0)
}
