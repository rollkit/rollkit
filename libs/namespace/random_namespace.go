package namespace

import tmrand "github.com/cometbft/cometbft/libs/rand"

func RandomNamespaceID() []byte {
	return tmrand.Bytes(NamespaceVersionZeroIDSize)
}

func RandomNamespace() Namespace {
	for {
		id := RandomNamespaceID()
		namespace := MustNewV0(id)
		err := namespace.ValidateBlobNamespace()
		if err != nil {
			continue
		}
		return namespace
	}
}

func RandomNamespaces(count int) (namespaces []Namespace) {
	for i := 0; i < count; i++ {
		namespaces = append(namespaces, RandomNamespace())
	}
	return namespaces
}
