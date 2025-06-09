package docker

import (
	tastoradocker "github.com/celestiaorg/tastora/framework/docker"
	tastoratypes "github.com/celestiaorg/tastora/framework/types"
	"testing"
)

func defaultRollkitProvider() tastoratypes.Provider {
	return tastoradocker.NewProvider()
}

func TestBasicDockerE2E(t *testing.T) {

}
func foo() {
	var _ = tastoratypes.DANode{}
}
