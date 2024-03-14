package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/signal"
	"syscall"

	proxy "github.com/rollkit/go-da/proxy/jsonrpc"
	goDATest "github.com/rollkit/go-da/test"
	"github.com/rollkit/rollkit/test/mocks"
)

func main() {
	var (
		host string
		port string
	)
	addr, _ := url.Parse(mocks.MockDAAddress)
	flag.StringVar(&port, "port", addr.Port(), "listening port")
	flag.StringVar(&host, "host", addr.Hostname(), "listening address")
	flag.Parse()

	srv := proxy.NewServer(host, port, goDATest.NewDummyDA())
	log.Printf("Listening on: %s:%s", host, port)
	if err := srv.Start(context.Background()); err != nil {
		log.Fatal("error while serving:", err)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGTERM)
	<-interrupt
	fmt.Println("\nCtrl+C pressed. Exiting...")
	os.Exit(0)
}
