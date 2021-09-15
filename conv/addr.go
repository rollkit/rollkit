package conv

import (
	"strings"

	"github.com/multiformats/go-multiaddr"

	"github.com/celestiaorg/optimint/config"
)

// TranslateAddresses updates conf by changing Cosmos-style addresses to Multiaddr format.
func TranslateAddresses(conf *config.NodeConfig) error {
	if conf.P2P.ListenAddress != "" {
		addr, err := GetMultiAddr(conf.P2P.ListenAddress)
		if err != nil {
			return err
		}
		conf.P2P.ListenAddress = addr.String()
	}

	seeds := strings.Split(conf.P2P.Seeds, ",")
	for i, seed := range seeds {
		if seed != "" {
			addr, err := GetMultiAddr(seed)
			if err != nil {
				return err
			}
			seeds[i] = addr.String()
		}
	}
	conf.P2P.Seeds = strings.Join(seeds, ",")

	return nil
}

// GetMultiAddr converts single Cosmos-style network address into Multiaddr.
// Input format: [protocol://][<NODE_ID>@]<IPv4>:<PORT>
func GetMultiAddr(addr string) (multiaddr.Multiaddr, error) {
	var err error
	var p2pID multiaddr.Multiaddr
	parts := strings.Split(addr, "://")
	proto := "tcp"
	if len(parts) == 2 {
		proto = parts[0]
		addr = parts[1]
	}

	if at := strings.IndexRune(addr, '@'); at != -1 {
		p2pID, err = multiaddr.NewMultiaddr("/p2p/" + addr[:at])
		if err != nil {
			return nil, err
		}
		addr = addr[at+1:]
	}
	parts = strings.Split(addr, ":")
	if len(parts) != 2 {
		return nil, errInvalidAddress
	}
	maddr, err := multiaddr.NewMultiaddr("/ip4/" + parts[0] + "/" + proto + "/" + parts[1])
	if err != nil {
		return nil, err
	}
	if p2pID != nil {
		maddr = maddr.Encapsulate(p2pID)
	}
	return maddr, nil
}
