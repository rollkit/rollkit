package cnrc

import (
	"context"
	"encoding/hex"
	"fmt"
	"github.com/go-resty/resty/v2"
	"strconv"
)

type Client struct {
	c *resty.Client
}

func NewClient(baseURL string, options ...Option) (*Client, error) {
	c := &Client{
		c: resty.New(),
	}

	c.c.SetBaseURL(baseURL)
	c.c.SetAllowGetMethodPayload(true)

	for _, option := range options {
		if err := option(c); err != nil {
			return nil, err
		}
	}

	return c, nil
}

func (c *Client) Header(ctx context.Context, height uint64) /* Header */ error {
	resp, err := c.c.R().
		SetContext(ctx).
		SetPathParam(heightKey, strconv.FormatUint(height, 10)).
		Get(headerPath())
	fmt.Println(resp, err)
	return err
}

func (c *Client) Balance(ctx context.Context) error {
	panic("Balance not implemented")
	return nil
}

func (c *Client) SubmitTx(ctx context.Context, tx []byte) /* TxResponse */ error {
	panic("SubmitTx not implemented")
	return nil
}

func (c *Client) SubmitPFD(ctx context.Context, namespaceID [8]byte, data []byte, gasLimit uint64) /* TxResponse */ error {
	panic("SubmitPFD not implemented")
	return nil
}

func (c *Client) NamespacedShares(ctx context.Context, namespaceID [8]byte, height uint64) ([][]byte, error) {
	req := SharesByNamespaceRequest{
		NamespaceID: hex.EncodeToString(namespaceID[:]),
		Height:      height,
	}
	var res [][]byte
	_, err := c.c.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&res).
		Get(namespacedSharesEndpoint)
	if err != nil {
		return nil, err
	}

	return res, nil
}

func headerPath() string {
	return fmt.Sprintf("%s/{%s}", headerEndpoint, heightKey)
}
