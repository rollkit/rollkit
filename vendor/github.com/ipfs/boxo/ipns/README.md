## Usage

To create a new IPNS record:

```go
import (
	"time"

	ipns "github.com/ipfs/boxo/ipns"
	crypto "github.com/libp2p/go-libp2p/core/crypto"
)

// Generate a private key to sign the IPNS record with. Most of the time,
// however, you'll want to retrieve an already-existing key from IPFS using the
// go-ipfs/core/coreapi CoreAPI.KeyAPI() interface.
privateKey, publicKey, err := crypto.GenerateKeyPair(crypto.RSA, 2048)
if err != nil {
  panic(err)
}

// Create an IPNS record that expires in one hour and points to the IPFS address
// /ipfs/Qme1knMqwt1hKZbc1BmQFmnm9f36nyQGwXxPGVpVJ9rMK5
ipnsRecord, err := ipns.Create(privateKey, []byte("/ipfs/Qme1knMqwt1hKZbc1BmQFmnm9f36nyQGwXxPGVpVJ9rMK5"), 0, time.Now().Add(1*time.Hour))
if err != nil {
	panic(err)
}
```

Once you have the record, you’ll need to use IPFS to *publish* it.

There are several other major operations you can do with `go-ipns`. Check out the [API docs](https://pkg.go.dev/github.com/ipfs/boxo/ipns) or look at the tests in this repo for examples.
