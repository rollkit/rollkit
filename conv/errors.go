package conv

import "errors"

var (
	ErrInvalidAddress = errors.New("invalid address format, expected [<NODE_ID>@]<IPv4>:<PORT>")
)
