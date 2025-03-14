# Secure FileSystem Signer

This package provides a secure implementation of a signer that stores cryptographic keys on the filesystem. The implementation follows security best practices to ensure that private keys are never stored in plain text on disk.

## Features

- **Secure Key Storage**: Private keys are encrypted using AES-GCM before being stored on disk
- **Passphrase Protection**: Keys are protected by a user-provided passphrase
- **In-Memory Operation**: Once loaded, keys remain in memory for signing operations
- **Automatic Key Generation**: If keys don't exist, they are automatically generated
- **Thread Safety**: Concurrent access to the signer is protected by mutex locks

## Security Measures

1. **Encryption**: Private keys are encrypted using AES-GCM, a secure authenticated encryption mode
2. **Permissions**: Key files are stored with 0600 permissions (readable and writable only by the owner)
3. **Directory Creation**: Key directories are created with 0700 permissions
4. **No Plain Text**: Private keys are never stored in plain text on disk
5. **Passphrase Derivation**: A key derivation function is used to derive encryption keys from passphrases

## Usage

```go
// Create a new signer (or load existing keys)
passphrase := []byte("your-secure-passphrase")
keyPath := "/path/to/secure/keys.json"
signer, err := NewFileSystemSigner(keyPath, passphrase)
if err != nil {
    // Handle error
}

// Sign a message
message := []byte("Message to sign")
signature, err := signer.Sign(message)
if err != nil {
    // Handle error
}

// Get the public key
pubKey, err := signer.GetPublic()
if err != nil {
    // Handle error
}
```

## Production Considerations

For production use, consider the following enhancements:

1. **Stronger KDF**: Replace the simple key derivation function with Argon2 or PBKDF2
2. **Secure Passphrase Handling**: Implement secure methods for obtaining and handling passphrases
3. **Memory Protection**: Add memory protection to prevent keys from being swapped to disk
4. **Key Rotation**: Implement key rotation policies
5. **Hardware Security**: For high-security environments, consider using hardware security modules (HSMs)

## Implementation Details

The `FileSystemSigner` stores keys in a JSON file with the following structure:

```json
{
  "priv_key_encrypted": "...", // Base64-encoded encrypted private key
  "nonce": "...",             // Base64-encoded nonce for AES-GCM
  "pub_key": "..."            // Base64-encoded public key
}
```

The encryption process:

1. Generate a random nonce
2. Derive an encryption key from the passphrase
3. Encrypt the private key using AES-GCM with the derived key and nonce
4. Store the encrypted private key, nonce, and public key in the JSON file

The decryption process:

1. Read the JSON file
2. Derive the encryption key from the passphrase
3. Decrypt the private key using AES-GCM with the derived key and stored nonce
4. Load the keys into memory for use
