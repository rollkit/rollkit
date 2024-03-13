package main

import (
	"context"
	"flag"
	"log"
	"net/url"

	"github.com/rollkit/go-da/proxy-jsonrpc"
	goDATest "github.com/rollkit/go-da/test"
)

const (
	// MockDAAddress is the mock address for the gRPC server
	MockDAAddress = "grpc://localhost:7980"
)

func main() {
	var (
		host string
		port string
	)
	addr, _ := url.Parse(MockDAAddress)
	flag.StringVar(&port, "port", addr.Port(), "listening port")
	flag.StringVar(&host, "host", addr.Hostname(), "listening address")
	flag.Parse()

	srv := proxy.NewServer(host, port, goDATest.NewDummyDA())
	log.Printf("Listening on: %s:%s", host, port)
	if err := srv.Start(context.Background()); err != nil {
		log.Fatal("error while serving:", err)
	}
}
