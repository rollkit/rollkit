package store

import (
	"strconv"

	"github.com/ipfs/go-datastore"

	"github.com/celestiaorg/go-header"
)

var (
	headKey = datastore.NewKey("head")
)

func heightKey(h uint64) datastore.Key {
	return datastore.NewKey(strconv.Itoa(int(h)))
}

func headerKey[H header.Header[H]](h H) datastore.Key {
	return datastore.NewKey(h.Hash().String())
}
