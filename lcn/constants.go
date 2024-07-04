package lcn

import "github.com/LastL2/cuberollkit/lcn/types"

// lcn return codes
const (
	StatusUnknown types.StatusCode = iota
	StatusSuccess
	StatusTimeout
	StatusError
)
