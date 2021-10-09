package store

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestPrefixKV(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)
	require := require.New(t)

	base := NewDefaultInMemoryKVStore()

	p1 := NewPrefixKV(base, []byte{1})
	p2 := NewPrefixKV(base, []byte{2})

	key1 := []byte("key1")
	key2 := []byte("key2")

	val11 := []byte("val11")
	val21 := []byte("val21")
	val12 := []byte("val12")
	val22 := []byte("val22")

	// set different values in each preffix
	err := p1.Set(key1, val11)
	require.NoError(err)

	err = p1.Set(key2, val12)
	require.NoError(err)

	err = p2.Set(key1, val21)
	require.NoError(err)

	err = p2.Set(key2, val22)
	require.NoError(err)

	// ensure that each PrefixKV returns proper data
	v, err := p1.Get(key1)
	require.NoError(err)
	assert.Equal(val11, v)

	v, err = p2.Get(key1)
	require.NoError(err)
	assert.Equal(val21, v)

	v, err = p1.Get(key2)
	require.NoError(err)
	assert.Equal(val12, v)

	v, err = p2.Get(key2)
	require.NoError(err)
	assert.Equal(val22, v)

	// delete from one prefix, ensure that second contains data
	err = p1.Delete(key1)
	require.NoError(err)

	err = p1.Delete(key2)
	require.NoError(err)

	v, err = p2.Get(key1)
	require.NoError(err)
	assert.Equal(val21, v)

	v, err = p2.Get(key2)
	require.NoError(err)
	assert.Equal(val22, v)
}

func TestPrefixKVBatch(t *testing.T) {
	t.Parallel()

	assert := assert.New(t)
	require := require.New(t)

	basekv := NewDefaultInMemoryKVStore()
	prefixkv := NewPrefixKV(basekv, []byte("prefix1"))
	prefixbatchkv1 := prefixkv.NewBatch()

	keys := [][]byte{[]byte("key1"), []byte("key2"), []byte("key3"), []byte("key4")}
	values := [][]byte{[]byte("value1"), []byte("value2"), []byte("value3"), []byte("value4")}

	for i := 0; i < len(keys); i++ {
		err := prefixbatchkv1.Set(keys[i], values[i])
		require.NoError(err)
	}

	err := prefixbatchkv1.Commit()
	require.NoError(err)

	for i := 0; i < len(keys); i++ {
		vals, err := prefixkv.Get(keys[i])
		assert.Equal(vals, values[i])
		require.NoError(err)
	}

	prefixbatchkv2 := prefixkv.NewBatch()
	err = prefixbatchkv2.Delete([]byte("key1"))
	require.NoError(err)

}
