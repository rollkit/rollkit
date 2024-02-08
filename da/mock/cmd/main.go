package main

import (
	"flag"
	"log"
	"net"
	"strconv"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/rollkit/go-da/proxy"
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

	lis, err := net.Listen("tcp", host+":"+strconv.Itoa(port))
	if err != nil {
		log.Panic(err)
	}
	log.Println("Listening on:", lis.Addr())
	srv := proxy.NewServer(goDATest.NewDummyDA(), grpc.Creds(insecure.NewCredentials()))
	if err := srv.Serve(lis); err != nil {
		log.Fatal("error while serving:", err)
	}
}
