package conv

import (
	"strings"

	"github.com/multiformats/go-multiaddr"
)

func GetMultiAddr(addr string) (multiaddr.Multiaddr, error) {
	var err error
	var p2pId multiaddr.Multiaddr
	if at := strings.IndexRune(addr, '@'); at != -1 {
		p2pId, err = multiaddr.NewMultiaddr("/p2p/" + addr[:at])
		if err != nil {
			return nil, err
		}
		addr = addr[at+1:]
	}
	parts := strings.Split(addr, ":")
	if len(parts) != 2 {
		return nil, ErrInvalidAddress
	}
	maddr, err := multiaddr.NewMultiaddr("/ip4/" + parts[0] + "/tcp/" + parts[1])
	if err != nil {
		return nil, err
	}
	if p2pId != nil {
		maddr = maddr.Encapsulate(p2pId)
	}
	return maddr, nil
}
