package conv

import "errors"

var (
	errInvalidAddress = errors.New("invalid address format, expected [protocol://][<NODE_ID>@]<IPv4>:<PORT>")
)
