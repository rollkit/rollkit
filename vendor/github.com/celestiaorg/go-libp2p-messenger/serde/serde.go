// This library provides utilities for length-prefixed de/serialization for messages with user-specific serialization
// format.
package serde

import (
	"encoding/binary"
	"fmt"
	"io"

	pool "github.com/libp2p/go-buffer-pool"
)

// TODO: 1mb is reasonable, but it still makes sense to make it configurable per use-case, rather than globally
// MaxMessageSize limits allowed size for messages that can de/serialized.
var MaxMessageSize uint64 = 1 << 20

// ErrMsgTooBig is signaled in case user tries to de/serialize message with size bigger than MaxMessageSize.
var ErrMsgTooBig = fmt.Errorf("serde: message exceeds max allowed size")

// Message defines an abstract Message to be de/serialized using arbitrary format.
// NOTE: Currently, Message is aligned and tested for protobuf(gogo/proto) support in the first place.
type Message interface {
	// Size reports the size of a Message.
	Size() int

	// MarshalTo serializes a Message to the given slice.
	// It also reports amount of bytes written.
	MarshalTo([]byte) (int, error)

	// Unmarshal deserialized a Message from a given slice.
	Unmarshal([]byte) error
}

// Marshal serializes the given Message into the given buffer with prefixed-length.
func Marshal(msg Message, buf []byte) (n int, err error) {
	s := uint64(msg.Size())
	if s > MaxMessageSize {
		return n, ErrMsgTooBig
	}

	// TODO: Check if buf is too small?
	n = binary.PutUvarint(buf, s)
	nn, err := msg.MarshalTo(buf[n:])
	n += nn
	if err != nil {
		return
	}

	return
}

// Unmarshal deserializes the given Message from the given data buffer.
// The data buffer should be filled using Marshal.
func Unmarshal(msg Message, data []byte) (n int, err error) {
	size, n := binary.Uvarint(data)
	if n < 0 {
		return 0, fmt.Errorf("serde: varint overflow")
	}
	if size > MaxMessageSize {
		return n, ErrMsgTooBig
	}

	nn := n + int(size)
	err = msg.Unmarshal(data[n:nn])
	if err != nil {
		return
	}

	return nn, nil
}

// Write marshals the given msg and writes it to the given Writer.
// It efficiently reuses allocated buffer with buffer pooling.
func Write(w io.Writer, msg Message) (n int, err error) {
	s := msg.Size()
	buf := pool.Get(uvarintSize(uint64(s)) + s)
	defer pool.Put(buf)

	n, err = Marshal(msg, buf)
	if err != nil {
		return
	}

	return w.Write(buf[:n])
}

// Read deserializes the given msg from the given Reader.
// It efficiently reuses allocated buffer with buffer pooling.
func Read(r io.Reader, msg Message) (n int, err error) {
	br, ok := r.(io.ByteReader)
	if !ok {
		br = NewByteReader(r)
	}

	size, err := binary.ReadUvarint(&byteCounter{br, &n})
	if err != nil {
		return
	}
	if size > MaxMessageSize {
		return n, ErrMsgTooBig
	}

	buf := pool.Get(int(size))
	nn, err := readWith(r, msg, buf)
	n += nn
	pool.Put(buf)
	return
}

func readWith(r io.Reader, msg Message, buf []byte) (int, error) {
	n, err := io.ReadFull(r, buf)
	if err != nil {
		return n, err
	}

	return n, msg.Unmarshal(buf)
}
