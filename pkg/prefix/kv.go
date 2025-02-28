package prefix

import (
	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
)

// NewPrefixKV creates a new prefix KV store
func NewPrefixKV(kvStore ds.Batching, prefix string) ds.Batching {
	return (ktds.Wrap(kvStore, ktds.PrefixTransform{Prefix: ds.NewKey(prefix)}).Children()[0]).(ds.Batching)
}
