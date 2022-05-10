package cnrc

// SubmitPFDRequest represents a request to submit a PayForData transaction.
type SubmitPFDRequest struct {
	NamespaceID string `json:"namespace_id"`
	Data        string `json:"data"`
	GasLimit    uint64 `json:"gas_limit"`
}

// SharesByNamespaceRequest represents a `GetSharesByNamespace`
// request payload
type SharesByNamespaceRequest struct {
	NamespaceID string `json:"namespace_id"`
	Height      uint64 `json:"height"`
}
