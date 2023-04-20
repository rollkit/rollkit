package main

import (
	"flag"
	"log"
	"net"
	"os"
	"strconv"

	cmlog "github.com/cometbft/cometbft/libs/log"

	grpcda "github.com/rollkit/rollkit/da/grpc"
	"github.com/rollkit/rollkit/da/grpc/mockserv"
	"github.com/rollkit/rollkit/store"
)

func main() {
	conf := grpcda.DefaultConfig
	logger := cmlog.NewTMLogger(os.Stdout)

	flag.IntVar(&conf.Port, "port", conf.Port, "listening port")
	flag.StringVar(&conf.Host, "host", "0.0.0.0", "listening address")
	flag.Parse()

	kv, err := store.NewDefaultKVStore(".", "db", "rollkit")
	if err != nil {
		log.Panic(err)
	}
	lis, err := net.Listen("tcp", conf.Host+":"+strconv.Itoa(conf.Port))
	if err != nil {
		log.Panic(err)
	}
	log.Println("Listening on:", lis.Addr())
	srv := mockserv.GetServer(kv, conf, nil, logger)
	if err := srv.Serve(lis); err != nil {
		log.Println("error while serving:", err)
	}
}
