package mock

import (
	"bytes"
	"encoding/binary"
)

// This code is extracted from celestia-app. It's here to build shares from messages (serialized blocks).
// TODO(tzdybal): if we stop using `/namespaced_shares` we can get rid of this file.

const (
	shareSize     = 256
	namespaceSize = 8
	msgShareSize  = shareSize - namespaceSize
)

// splitMessage breaks the data in a message into the minimum number of
// namespaced shares
func splitMessage(rawData []byte, nid []byte) []NamespacedShare {
	shares := make([]NamespacedShare, 0)
	firstRawShare := append(append(
		make([]byte, 0, shareSize),
		nid...),
		rawData[:msgShareSize]...,
	)
	shares = append(shares, NamespacedShare{firstRawShare, nid})
	rawData = rawData[msgShareSize:]
	for len(rawData) > 0 {
		shareSizeOrLen := min(msgShareSize, len(rawData))
		rawShare := append(append(
			make([]byte, 0, shareSize),
			nid...),
			rawData[:shareSizeOrLen]...,
		)
		paddedShare := zeroPadIfNecessary(rawShare, shareSize)
		share := NamespacedShare{paddedShare, nid}
		shares = append(shares, share)
		rawData = rawData[shareSizeOrLen:]
	}
	return shares
}

// Share contains the raw share data without the corresponding namespace.
type Share []byte

// NamespacedShare extends a Share with the corresponding namespace.
type NamespacedShare struct {
	Share
	ID []byte
}

func min(a, b int) int {
	if a <= b {
		return a
	}
	return b
}

func zeroPadIfNecessary(share []byte, width int) []byte {
	oldLen := len(share)
	if oldLen < width {
		missingBytes := width - oldLen
		padByte := []byte{0}
		padding := bytes.Repeat(padByte, missingBytes)
		share = append(share, padding...)
		return share
	}
	return share
}

// marshalDelimited marshals the raw data (excluding the namespace) of this
// message and prefixes it with the length of that encoding.
func marshalDelimited(data []byte) ([]byte, error) {
	lenBuf := make([]byte, binary.MaxVarintLen64)
	length := uint64(len(data))
	n := binary.PutUvarint(lenBuf, length)
	return append(lenBuf[:n], data...), nil
}

// appendToShares appends raw data as shares.
// Used to build shares from blocks/messages.
func appendToShares(shares []NamespacedShare, nid []byte, rawData []byte) []NamespacedShare {
	if len(rawData) <= msgShareSize {
		rawShare := append(append(
			make([]byte, 0, len(nid)+len(rawData)),
			nid...),
			rawData...,
		)
		paddedShare := zeroPadIfNecessary(rawShare, shareSize)
		share := NamespacedShare{paddedShare, nid}
		shares = append(shares, share)
	} else { // len(rawData) > msgShareSize
		shares = append(shares, splitMessage(rawData, nid)...)
	}
	return shares
}

type namespacedSharesResponse struct {
	Shares []Share `json:"shares"`
	Height uint64  `json:"height"`
}

type namespacedDataResponse struct {
	Data   [][]byte `json:"data"`
	Height uint64   `json:"height"`
}
