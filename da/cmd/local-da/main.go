package main

import (
	"context"
	"flag"
	"fmt"

	"os"
	"os/signal"
	"syscall"

	"cosmossdk.io/log"

	proxy "github.com/rollkit/rollkit/da/jsonrpc"
)

const (
	defaultHost = "localhost"
	defaultPort = "7980"
)

func main() {
	var (
		host      string
		port      string
		listenAll bool
	)
	flag.StringVar(&port, "port", defaultPort, "listening port")
	flag.StringVar(&host, "host", defaultHost, "listening address")
	flag.BoolVar(&listenAll, "listen-all", false, "listen on all network interfaces (0.0.0.0) instead of just localhost")
	flag.Parse()

	if listenAll {
		host = "0.0.0.0"
	}

	// create logger
	logger := log.NewLogger(os.Stdout).With("module", "da")
	da := NewLocalDA(logger)

	srv := proxy.NewServer(logger, host, port, da)
	logger.Info("Listening on", host, port)
	if err := srv.Start(context.Background()); err != nil {
		logger.Info("error while serving:", err)
	}

	interrupt := make(chan os.Signal, 1)
	signal.Notify(interrupt, os.Interrupt, syscall.SIGINT)
	<-interrupt
	fmt.Println("\nCtrl+C pressed. Exiting...")
	os.Exit(0)
}
