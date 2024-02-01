package peers

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

// Peer encodes connection information for a peer
type Peer struct {
	IP   net.IP
	Port uint16
}

// Unmarshal parses peer ip addresses and ports from a buffer
func Unmarshal(peerBin []byte) ([]Peer, error) {
	const peerSize = 6
	numPeers := len(peerBin) / peerSize
	if len(peerBin)%peerSize != 0 {
		err := fmt.Errorf("received malformed peers")
		return nil, err
	}
	peers := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {
		offset := i * peerSize
		peers[i] = Peer{
			IP:   peerBin[offset : offset+4],
			Port: binary.BigEndian.Uint16(peerBin[offset+4 : offset+6]),
		}
	}
	return peers, nil
}

func (p Peer) String() string {
	return net.JoinHostPort(p.IP.String(), strconv.Itoa(int(p.Port)))
}
