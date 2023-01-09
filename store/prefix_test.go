package store

import (
	"context"
	"testing"

	ds "github.com/ipfs/go-datastore"
	ktds "github.com/ipfs/go-datastore/keytransform"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixKV1(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)
	require := require.New(t)

	ctx := context.Background()
	base, _ := NewDefaultInMemoryKVStore()

	p1 := ktds.Wrap(base, ktds.PrefixTransform{Prefix: ds.NewKey("1")})
	p2 := ktds.Wrap(base, ktds.PrefixTransform{Prefix: ds.NewKey("2")})

	key1 := ds.NewKey("key1")
	key2 := ds.NewKey("key2")

	val11 := []byte("val11")
	val21 := []byte("val21")
	val12 := []byte("val12")
	val22 := []byte("val22")

	// set different values in each preffix
	err := p1.Put(ctx, key1, val11)
	require.NoError(err)

	err = p1.Put(ctx, key2, val12)
	require.NoError(err)

	err = p2.Put(ctx, key1, val21)
	require.NoError(err)

	err = p2.Put(ctx, key2, val22)
	require.NoError(err)

	// ensure that each PrefixKV returns proper data
	v, err := p1.Get(ctx, key1)
	require.NoError(err)
	assert.Equal(val11, v)

	v, err = p2.Get(ctx, key1)
	require.NoError(err)
	assert.Equal(val21, v)

	v, err = p1.Get(ctx, key2)
	require.NoError(err)
	assert.Equal(val12, v)

	v, err = p2.Get(ctx, key2)
	require.NoError(err)
	assert.Equal(val22, v)

	// delete from one prefix, ensure that second contains data
	err = p1.Delete(ctx, key1)
	require.NoError(err)

	err = p1.Delete(ctx, key2)
	require.NoError(err)

	v, err = p2.Get(ctx, key1)
	require.NoError(err)
	assert.Equal(val21, v)

	v, err = p2.Get(ctx, key2)
	require.NoError(err)
	assert.Equal(val22, v)
}

func TestPrefixKVBatch(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)
	require := require.New(t)

	ctx := context.Background()

	basekv, _ := NewDefaultInMemoryKVStore()
	prefixkv := ktds.Wrap(basekv, ktds.PrefixTransform{Prefix: ds.NewKey("prefix1")}).Children()[0]

	badgerPrefixkv, _ := prefixkv.(ds.TxnDatastore)
	prefixbatchkv1, _ := badgerPrefixkv.NewTransaction(ctx, false)

	keys := []ds.Key{ds.NewKey("key1"), ds.NewKey("key2"), ds.NewKey("key3"), ds.NewKey("key4")}
	values := [][]byte{[]byte("value1"), []byte("value2"), []byte("value3"), []byte("value4")}

	for i := 0; i < len(keys); i++ {
		err := prefixbatchkv1.Put(ctx, keys[i], values[i])
		require.NoError(err)
	}

	err := prefixbatchkv1.Commit(ctx)
	require.NoError(err)

	for i := 0; i < len(keys); i++ {
		vals, err := prefixkv.Get(ctx, keys[i])
		assert.Equal(vals, values[i])
		require.NoError(err)
	}

	prefixbatchkv2, _ := badgerPrefixkv.NewTransaction(ctx, false)
	err = prefixbatchkv2.Delete(ctx, ds.NewKey("key1"))
	require.NoError(err)

}
