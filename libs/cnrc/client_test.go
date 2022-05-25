package cnrc

import (
	"context"
	"errors"
	"github.com/go-resty/resty/v2"
	"github.com/ory/dockertest"
	"github.com/stretchr/testify/assert"
	"log"
	"net/http"
	"os"
	"testing"
	"time"
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

// dockertest should be used to spin up ephemeral-cluster and expose celestia-node RPC endpoint.
var pool *dockertest.Pool

func TestMain(m *testing.M) {
	var err error
	pool, err = dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
		os.Exit(1)
	}

	code := m.Run()
	os.Exit(code)
}

func setupDockerNode() (*dockertest.Resource, error) {
	opts := &dockertest.RunOptions{
		Hostname:     "node1",
		Repository:   "ghcr.io/celestiaorg/celestia-node",
		Tag:          "sha-c436d6d",
		Cmd:          []string{"celestia", "light", "--node.store", "/celestia-light", "start", "--rpc.port", "26658"},
		Env:          []string{"NODE_TYPE=light"},
		ExposedPorts: []string{"26658/tcp"},
	}
	res, err := pool.RunWithOptions(opts)
	if err != nil {
		return nil, err
	}
	err = pool.Retry(func() error {
		port := res.GetPort("26658/tcp")
		log.Println("celestia RPC at: ", port)
		client := resty.New()
		res, err := client.R().Get("http://localhost:" + port)
		if err != nil {
			log.Println(err)
			return err
		}
		if res.IsError() && res.StatusCode() != http.StatusNotFound {
			return errors.New("")
		}
		if port == "" {
			return errors.New("waiting...")
		}
		return nil
	})
	if err != nil {
		return nil, err
	}
	return res, nil
}
