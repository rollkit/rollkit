package serde

import "io"

type ByteReader struct {
	io.Reader

	b [1]byte
}

func NewByteReader(r io.Reader) *ByteReader {
	return &ByteReader{Reader: r}
}

func (b *ByteReader) ReadByte() (byte, error) {
	_, err := io.ReadFull(b.Reader, b.b[:])
	if err != nil {
		return 0, err
	}

	return b.b[0], nil
}

type byteCounter struct {
	br io.ByteReader
	i  *int
}

func (bc *byteCounter) ReadByte() (byte, error) {
	b, err := bc.br.ReadByte()
	if err != nil {
		return 0, err
	}

	*bc.i++
	return b, nil
}
