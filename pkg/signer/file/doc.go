/*
File Remote Signer implements the Signer interface using a file to store the keys.

The keys are stored in a file in the local filesystem.

	passphrase := []byte("your-secure-passphrase")
	keyPath := "/path/to/secure/keys.json"

	// Create or load a signer
	signer, err := NewFileSystemSigner(keyPath, passphrase)
	if err != nil {
		panic(err)
	}

	// Sign a message
	message := []byte("Message to sign")
	signature, err := signer.Sign(message)
	if err != nil {
		panic(err)
	}

	// Get the public key
	pubKey, err := signer.GetPublic()
	if err != nil {
		panic(err)
	}

	// Verify the signature (typically done by another party)
	valid, err := pubKey.Verify(message, signature)
	if err != nil {
		panic(err)
	}
*/
package file
