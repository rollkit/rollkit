package celestia

import (
	"bytes"
	"encoding/binary"
	"math"
)

// This file contains logic copied from DALC, used to reconstruct messages from shares.

// ParseMsgs collects all messages from the shares provided
func parseMsgs(shares [][]byte) (Messages, error) {
	msgList, err := parseMsgShares(shares)
	if err != nil {
		return Messages{}, err
	}

	return Messages{
		MessagesList: msgList,
	}, nil
}

// parseMsgShares iterates through raw shares and separates the contiguous chunks
// of data. It is only used for Messages, i.e. shares with a non-reserved namespace.
func parseMsgShares(shares [][]byte) ([]Message, error) {
	if len(shares) == 0 {
		return nil, nil
	}

	// set the first nid and current share
	nid := shares[0][:NamespaceSize]
	currentShare := shares[0][NamespaceSize:]
	// find and remove the msg len delimiter
	currentShare, msgLen, err := ParseDelimiter(currentShare)
	if err != nil {
		return nil, err
	}

	var msgs []Message
	for cursor := uint64(0); cursor < uint64(len(shares)); {
		var msg Message
		currentShare, nid, cursor, msgLen, msg, err = nextMsg(
			shares,
			currentShare,
			nid,
			cursor,
			msgLen,
		)
		if err != nil {
			return nil, err
		}
		if msg.Data != nil {
			msgs = append(msgs, msg)
		}
	}

	return msgs, nil
}

func nextMsg(
	shares [][]byte,
	current,
	nid []byte,
	cursor,
	msgLen uint64,
) ([]byte, []byte, uint64, uint64, Message, error) {
	switch {
	// the message uses all of the current share data and at least some of the
	// next share
	case msgLen > uint64(len(current)):
		// add the next share to the current one and try again
		cursor++
		current = append(current, shares[cursor][NamespaceSize:]...)
		return nextMsg(shares, current, nid, cursor, msgLen)

	// the msg we're looking for is contained in the current share
	case msgLen <= uint64(len(current)):
		msg := Message{nid, current[:msgLen]}
		cursor++

		// call it a day if the work is done
		if cursor >= uint64(len(shares)) {
			return nil, nil, cursor, 0, msg, nil
		}

		nextNid := shares[cursor][:NamespaceSize]
		next, msgLen, err := ParseDelimiter(shares[cursor][NamespaceSize:])
		return next, nextNid, cursor, msgLen, msg, err
	}
	// this code is unreachable but the compiler doesn't know that
	return nil, nil, 0, 0, Message{}, nil
}

// ParseDelimiter finds and returns the length delimiter of the message provided
// while also removing the delimiter bytes from the input
func ParseDelimiter(input []byte) ([]byte, uint64, error) {
	if len(input) == 0 {
		return input, 0, nil
	}

	l := binary.MaxVarintLen64
	if len(input) < binary.MaxVarintLen64 {
		l = len(input)
	}

	delimiter := zeroPadIfNecessary(input[:l], binary.MaxVarintLen64)

	// read the length of the message
	r := bytes.NewBuffer(delimiter)
	msgLen, err := binary.ReadUvarint(r)
	if err != nil {
		return nil, 0, err
	}

	// calculate the number of bytes used by the delimiter
	lenBuf := make([]byte, binary.MaxVarintLen64)
	n := binary.PutUvarint(lenBuf, msgLen)

	// return the input without the length delimiter
	return input[n:], msgLen, nil
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

const NamespaceSize = 8

type Messages struct {
	MessagesList []Message `json:"msgs"`
}

type Message struct {
	// NamespaceID defines the namespace of this message, i.e. the
	// namespace it will use in the namespaced Merkle tree.
	//
	// TODO: spec out constrains and
	// introduce dedicated type instead of just []byte
	NamespaceID ID

	// Data is the actual data contained in the message
	// (e.g. a block of a virtual sidechain).
	Data []byte
}

type ID []byte

func (nid ID) Less(other ID) bool {
	return bytes.Compare(nid, other) < 0
}

func (nid ID) Equal(other ID) bool {
	return bytes.Equal(nid, other)
}

func (nid ID) LessOrEqual(other ID) bool {
	return bytes.Compare(nid, other) <= 0
}

func (nid ID) Size() IDSize {
	return IDSize(len(nid))
}
func (nid ID) String() string {
	return string(nid)
}

// IDSize is the number of bytes a namespace uses.
// Valid values are in [0,255].
type IDSize uint8

// IDMaxSize defines the max. allowed namespace ID size in bytes.
const IDMaxSize = math.MaxUint8
