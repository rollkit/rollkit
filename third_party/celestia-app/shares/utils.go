package shares

import (
	"bytes"
	"encoding/binary"

	coretypes "github.com/cometbft/cometbft/types"

	"github.com/rollkit/rollkit/third_party/celestia-app/appconsts"
	appns "github.com/rollkit/rollkit/third_party/celestia-app/namespace"
)

// DelimLen calculates the length of the delimiter for a given unit size
func DelimLen(size uint64) int {
	lenBuf := make([]byte, binary.MaxVarintLen64)
	return binary.PutUvarint(lenBuf, size)
}

// func isPowerOf2(v uint64) bool {
// 	return v&(v-1) == 0 && v != 0
// }

// func BlobsFromProto(blobs []core.Blob) ([]coretypes.Blob, error) {
// 	result := make([]coretypes.Blob, len(blobs))
// 	for i, blob := range blobs {
// 		if blob.ShareVersion > math.MaxUint8 {
// 			return nil, fmt.Errorf("share version %d is too large to be a uint8", blob.ShareVersion)
// 		}
// 		result[i] = coretypes.Blob{
// 			NamespaceID:  blob.NamespaceId,
// 			Data:         blob.Data,
// 			ShareVersion: uint8(blob.ShareVersion),
// 		}
// 	}
// 	return result, nil
// }

func TxsToBytes(txs coretypes.Txs) [][]byte {
	e := make([][]byte, len(txs))
	for i, tx := range txs {
		e[i] = []byte(tx)
	}
	return e
}

func TxsFromBytes(txs [][]byte) coretypes.Txs {
	e := make(coretypes.Txs, len(txs))
	for i, tx := range txs {
		e[i] = coretypes.Tx(tx)
	}
	return e
}

// zeroPadIfNecessary pads the share with trailing zero bytes if the provided
// share has fewer bytes than width. Returns the share unmodified if the
// len(share) is greater than or equal to width.
func zeroPadIfNecessary(share []byte, width int) (padded []byte, bytesOfPadding int) {
	oldLen := len(share)
	if oldLen >= width {
		return share, 0
	}

	missingBytes := width - oldLen
	padByte := []byte{0}
	padding := bytes.Repeat(padByte, missingBytes)
	share = append(share, padding...)
	return share, missingBytes
}

// ParseDelimiter attempts to parse a varint length delimiter from the input
// provided. It returns the input without the len delimiter bytes, the length
// parsed from the varint optionally an error. Unit length delimiters are used
// in compact shares where units (i.e. a transaction) are prefixed with a length
// delimiter that is encoded as a varint. Input should not contain the namespace
// ID or info byte of a share.
func ParseDelimiter(input []byte) (inputWithoutLenDelimiter []byte, unitLen uint64, err error) {
	if len(input) == 0 {
		return input, 0, nil
	}

	l := binary.MaxVarintLen64
	if len(input) < binary.MaxVarintLen64 {
		l = len(input)
	}

	delimiter, _ := zeroPadIfNecessary(input[:l], binary.MaxVarintLen64)

	// read the length of the data
	r := bytes.NewBuffer(delimiter)
	dataLen, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, 0, err
	}

	// calculate the number of bytes used by the delimiter
	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(lenBuf, dataLen)

	// return the input without the length delimiter
	return input[n:], dataLen, nil
}

// ParseTxs collects all of the transactions from the shares provided
func ParseTxs(shares []Share) (coretypes.Txs, error) {
	// parse the shares
	rawTxs, err := parseCompactShares(shares, appconsts.SupportedShareVersions)
	if err != nil {
		return nil, err
	}

	// convert to the Tx type
	txs := make(coretypes.Txs, len(rawTxs))
	for i := 0; i < len(txs); i++ {
		txs[i] = coretypes.Tx(rawTxs[i])
	}

	return txs, nil
}

func SplitTxs(txs coretypes.Txs) (txShares []Share, err error) {
	txWriter := NewCompactShareSplitter(appns.TxNamespace, appconsts.ShareVersionZero)

	for _, tx := range txs {
		err = txWriter.WriteTx(tx)
		if err != nil {
			return nil, err
		}
	}

	txShares, _, err = txWriter.Export(0)
	if err != nil {
		return nil, err
	}

	return txShares, nil
}
