package mock

// This code is extracted from celestia-app. It's here to build shares from messages (serialized blocks).
// TODO(tzdybal): if we stop using `/namespaced_shares` we can get rid of this file.

// Share contains the raw share data without the corresponding namespace.
type Share []byte

// NamespacedShare extends a Share with the corresponding namespace.
type NamespacedShare struct {
	Share
	ID []byte
}
