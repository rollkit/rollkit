package main

import (
	"context"
	"flag"
	"log"
	"strconv"

	"github.com/rollkit/go-da/proxy-jsonrpc"
	goDATest "github.com/rollkit/go-da/test"
)

func main() {
	var (
		host string
		port int
	)
	flag.IntVar(&port, "port", 7980, "listening port")
	flag.StringVar(&host, "host", "0.0.0.0", "listening address")
	flag.Parse()

	srv := proxy.NewServer(host, strconv.Itoa(port), goDATest.NewDummyDA())
	log.Printf("Listening on: %s:%d", host, port)
	if err := srv.Start(context.Background()); err != nil {
		log.Fatal("error while serving:", err)
	}
}
