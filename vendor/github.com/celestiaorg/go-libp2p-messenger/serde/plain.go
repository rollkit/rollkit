package serde

import "bytes"

// PlainMessage is the simplest implementation for Message which only copies bytes as a serialization.
type PlainMessage struct {
	Data []byte
}

func (f *PlainMessage) Size() int {
	return len(f.Data)
}

func (f *PlainMessage) MarshalTo(buf []byte) (int, error) {
	return copy(buf, f.Data), nil
}

func (f *PlainMessage) Unmarshal(data []byte) error {
	f.Data = make([]byte, len(data))
	copy(f.Data, data)
	return nil
}

func (f *PlainMessage) Equals(to *PlainMessage) bool {
	return bytes.Equal(f.Data, to.Data)
}
