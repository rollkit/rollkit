package types

type PubKey struct {
	Bytes []byte
	Type  string
}

func (pk *PubKey) Address() []byte {
	panic("not implemented")
}
