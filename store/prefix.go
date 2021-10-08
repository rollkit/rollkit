package store

type PrefixKV struct {
	kv     KVStore
	prefix []byte
}

func NewPrefixKV(kv KVStore, prefix []byte) *PrefixKV {
	return &PrefixKV{
		kv:     kv,
		prefix: prefix,
	}
}

func (p *PrefixKV) Get(key []byte) ([]byte, error) {
	return p.kv.Get(append(p.prefix, key...))
}

func (p *PrefixKV) Set(key []byte, value []byte) error {
	return p.kv.Set(append(p.prefix, key...), value)
}

func (p *PrefixKV) Delete(key []byte) error {
	return p.kv.Delete(append(p.prefix, key...))
}
func (p *PrefixKV) NewBatch() Batch {
	panic("Not Implemented!")
}

var _ KVStore = &PrefixKV{}
