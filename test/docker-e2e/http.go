package docker_e2e

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
)

// Client is a simple HTTP client for testing.
type Client struct {
	baseURL string
}

// NewClient creates a new Client.
func NewClient(host, port string) (*Client, error) {
	return &Client{
		baseURL: fmt.Sprintf("http://%s:%s", host, port),
	}, nil
}

// Get sends a GET request to the given path.
func (c *Client) Get(ctx context.Context, path string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}

// Post sends a POST request to the given path with the given key and value.
func (c *Client) Post(ctx context.Context, path, key, value string) ([]byte, error) {
	body := strings.NewReader(fmt.Sprintf("%s=%s", key, value))
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "text/plain")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	return io.ReadAll(resp.Body)
}
