package headertest

import (
	"context"

	"github.com/celestiaorg/go-header"
)

type Subscriber[H header.Header[H]] struct {
	Headers []H
}

func NewDummySubscriber() *Subscriber[*DummyHeader] {
	return &Subscriber[*DummyHeader]{}
}

func (mhs *Subscriber[H]) SetVerifier(func(context.Context, H) error) error {
	return nil
}

func (mhs *Subscriber[H]) Subscribe() (header.Subscription[H], error) {
	return mhs, nil
}

func (mhs *Subscriber[H]) NextHeader(ctx context.Context) (H, error) {
	defer func() {
		if len(mhs.Headers) > 1 {
			// pop the already-returned header
			cp := mhs.Headers
			mhs.Headers = cp[1:]
		} else {
			mhs.Headers = make([]H, 0)
		}
	}()
	if len(mhs.Headers) == 0 {
		var zero H
		return zero, context.Canceled
	}
	return mhs.Headers[0], nil
}

func (mhs *Subscriber[H]) Stop(context.Context) error { return nil }
func (mhs *Subscriber[H]) Cancel()                    {}
