package p2p

import "errors"

var (
	ErrNoPrivKey      = errors.New("private key not provided")
	ErrInvalidAddress = errors.New("invalid address format, expected [<NODE_ID>@]<IPv4>:<PORT>")
)
