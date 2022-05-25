package cnrc

import (
	"context"
	"encoding/hex"
	"errors"
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
	return errors.New("method Balance not implemented")
}

func (c *Client) SubmitTx(ctx context.Context, tx []byte) /* TxResponse */ error {
	return errors.New("method SubmitTx not implemented")
}

func (c *Client) SubmitPFD(ctx context.Context, namespaceID [8]byte, data []byte, gasLimit uint64) (*TxResponse, error) {
	req := SubmitPFDRequest{
		NamespaceID: hex.EncodeToString(namespaceID[:]),
		Data:        hex.EncodeToString(data),
		GasLimit:    gasLimit,
	}
	var res TxResponse
	var rpcErr string
	_, err := c.c.R().
		SetContext(ctx).
		SetBody(req).
		SetResult(&res).
		SetError(&rpcErr).
		Post(submitPFDEndpoint)
	if err != nil {
		return nil, err
	}
	if rpcErr != "" {
		return nil, errors.New(rpcErr)
	}
	return &res, nil
}

func (c *Client) NamespacedShares(ctx context.Context, namespaceID [8]byte, height uint64) ([][]byte, error) {
	var res struct {
		Shares [][]byte `json:"shares"`
		Height uint64   `json:"height"`
	}

	err := c.callNamespacedEndpoint(ctx, namespaceID, height, namespacedSharesEndpoint, &res)
	if err != nil {
		return nil, err
	}

	return res.Shares, nil
}

func (c *Client) NamespacedData(ctx context.Context, namespaceID [8]byte, height uint64) ([][]byte, error) {
	var res struct {
		Data   [][]byte `json:"data"`
		Height uint64   `json:"height"`
	}

	err := c.callNamespacedEndpoint(ctx, namespaceID, height, namespacedDataEndpoint, &res)
	if err != nil {
		return nil, err
	}

	return res.Data, nil
}

// callNamespacedEndpoint fetches result of /namespaced_{type} family of endpoints into result (this should be pointer!)
func (c *Client) callNamespacedEndpoint(ctx context.Context, namespaceID [8]byte, height uint64, endpoint string, result interface{}) error {
	var rpcErr string
	_, err := c.c.R().
		SetContext(ctx).
		SetResult(result).
		SetError(&rpcErr).
		Get(namespacedPath(endpoint, namespaceID, height))
	if err != nil {
		return err
	}
	if rpcErr != "" {
		return errors.New(rpcErr)
	}
	return nil
}

func headerPath() string {
	return fmt.Sprintf("%s/%s", headerEndpoint, heightKey)
}

func namespacedPath(endpoint string, namespaceID [8]byte, height uint64) string {
	return fmt.Sprintf("%s/%s/height/%d", endpoint, hex.EncodeToString(namespaceID[:]), height)
}
