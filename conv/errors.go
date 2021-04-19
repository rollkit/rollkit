package conv

import "errors"

var (
	ErrInvalidAddress = errors.New("invalid address format, expected [protocol://][<NODE_ID>@]<IPv4>:<PORT>")
)
