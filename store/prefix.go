package store

var _ KVStore = &PrefixKV{}
var _ Batch = &PrefixKVBatch{}

// PrefixKV is a key-value store that prepends all keys with given prefix.
type PrefixKV struct {
	kv     KVStore
	prefix []byte
}

// NewPrefixKV creates new PrefixKV on top of other KVStore.
func NewPrefixKV(kv KVStore, prefix []byte) *PrefixKV {
	return &PrefixKV{
		kv:     kv,
		prefix: prefix,
	}
}

// Get returns value for given key.
func (p *PrefixKV) Get(key []byte) ([]byte, error) {
	return p.kv.Get(append(p.prefix, key...))
}

// Set updates the value for given key.
func (p *PrefixKV) Set(key []byte, value []byte) error {
	return p.kv.Set(append(p.prefix, key...), value)
}

// Delete deletes key-value pair for given key.
func (p *PrefixKV) Delete(key []byte) error {
	return p.kv.Delete(append(p.prefix, key...))
}

// NewBatch creates a new batch.
func (p *PrefixKV) NewBatch() Batch {
	return &PrefixKVBatch{
		b:      p.kv.NewBatch(),
		prefix: p.prefix,
	}
}

// PrefixIterator creates iterator to traverse given prefix.
func (p *PrefixKV) PrefixIterator(prefix []byte) Iterator {
	return p.kv.PrefixIterator(append(p.prefix, prefix...))
}

// PrefixKVBatch enables batching of operations on PrefixKV.
type PrefixKVBatch struct {
	b      Batch
	prefix []byte
}

// Set adds key-value pair to batch.
func (pb *PrefixKVBatch) Set(key, value []byte) error {
	return pb.b.Set(append(pb.prefix, key...), value)
}

// Delete adds delete operation to batch.
func (pb *PrefixKVBatch) Delete(key []byte) error {
	return pb.b.Delete(append(pb.prefix, key...))
}

// Commit applies all operations in the batch atomically.
func (pb *PrefixKVBatch) Commit() error {
	return pb.b.Commit()
}

// Discard discards all operations in the batch.
func (pb *PrefixKVBatch) Discard() {
	pb.b.Discard()
}
