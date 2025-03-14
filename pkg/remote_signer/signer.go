package remote_signer

type Signer interface {
	// Sign takes a message as bytes and returns its signature.
	Sign(message []byte) ([]byte, error)

	// GetPublic returns the public key paired with this private key.
	GetPublic() ([]byte, error)
}
