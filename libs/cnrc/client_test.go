package cnrc

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestNewClient(t *testing.T) {
	cases := []struct {
		name          string
		options       []Option
		expectedError error
	}{
		{"without options", nil, nil},
		{"with timeout", []Option{WithTimeout(1 * time.Second)}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			client, err := NewClient("", c.options...)
			assert.ErrorIs(t, err, c.expectedError)
			if c.expectedError != nil {
				assert.Nil(t, client)
			} else {
				assert.NotNil(t, client)
			}
		})
	}
}

func TestNamespacedShares(t *testing.T) {
	t.Skip()
	client, err := NewClient("http://localhost:26658")
	assert.NoError(t, err)
	assert.NotNil(t, client)

	shares, err := client.NamespacedShares(context.TODO(), [8]byte{1, 2, 3, 4, 5, 6, 7, 8}, 8)
	assert.NoError(t, err)
	assert.NotNil(t, shares)
	assert.Len(t, shares, 4)
}

func TestSubmitPDF(t *testing.T) {
	t.Skip()
	client, err := NewClient("http://localhost:26658", WithTimeout(30*time.Second))
	assert.NoError(t, err)
	assert.NotNil(t, client)

	txRes, err := client.SubmitPFD(context.TODO(), [8]byte{1, 2, 3, 4, 5, 6, 7, 8}, []byte("random data"), 100000)
	assert.NoError(t, err)
	assert.NotNil(t, txRes)
}
