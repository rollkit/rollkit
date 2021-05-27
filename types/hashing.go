package types

import "crypto/sha256"

func Hash(object Serializable) ([32]byte, error) {
	blob, err := object.Serialize()
	if err != nil {
		return [32]byte{}, err
	}
	return sha256.Sum256(blob), nil
}
