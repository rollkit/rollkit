package block

import "github.com/rollkit/rollkit/types"

// ManagerOption is a function that configures the Manager.
type ManagerOption func(*Manager)

// WithSignaturePayloadProvider sets the signature payload provider for the manager.
func WithSignaturePayloadProvider(provider types.SignaturePayloadProvider) ManagerOption {
	return func(m *Manager) {
		m.signaturePayloadProvider = provider
	}
}
