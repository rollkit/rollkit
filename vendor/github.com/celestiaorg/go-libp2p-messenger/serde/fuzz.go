//go:build gofuzz
// +build gofuzz

package serde

import "io"

func Fuzz(data []byte) int {
	rw := &testRW{}
	in := &fakeMsg{data: data}

	n, err := Write(rw, in)
	if err != nil {
		return 0
	}

	out := &fakeMsg{}
	nn, err := Read(rw, out)
	if err != nil {
		return 0
	}

	if n != nn {
		panic("read/write not equal")
	}
	return 1
}

type testRW struct {
	buf  []byte
	r, w int // read, written
}

func (rw *testRW) Write(b []byte) (n int, err error) {
	if len(rw.buf) == rw.w {
		rw.buf = append(rw.buf, make([]byte, len(b))...)
	}
	n = copy(rw.buf[rw.w:], b)
	rw.w += n
	return
}

func (rw *testRW) Read(b []byte) (n int, err error) {
	if len(rw.buf) == rw.r {
		return 0, io.EOF
	}
	n = copy(b, rw.buf[rw.r:])
	rw.r += n
	return
}

type fakeMsg struct {
	data []byte
}

func (f *fakeMsg) Size() int {
	return len(f.data)
}

func (f *fakeMsg) MarshalTo(buf []byte) (int, error) {
	return copy(buf, f.data), nil
}

func (f *fakeMsg) Unmarshal(data []byte) error {
	f.data = make([]byte, len(data))
	copy(f.data, data)
	return nil
}
