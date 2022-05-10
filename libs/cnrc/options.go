package cnrc

import "time"

type Option func(*Client) error

func WithTimeout(timeout time.Duration) Option {
	return func(c *Client) error {
		c.c.SetTimeout(timeout)
		return nil
	}
}
