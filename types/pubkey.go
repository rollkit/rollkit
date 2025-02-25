package types

// PubKey is a type for a public key.
type PubKey struct {
	Bytes []byte
	Type  string
}

// Address returns the address of the public key.
func (pk *PubKey) Address() []byte {
	panic("not implemented")
}
