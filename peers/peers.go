package peers

import (
	"encoding/binary"
	"fmt"
	"net"
	"strconv"
)

type Peer struct {
	PeerID string `bencode:"peer id"`
	IP     string `bencode:"ip"`
	Port   uint16 `bencode:"port"`
}

func (p *Peer) String() string {
	return net.JoinHostPort(p.IP, p.PortStr())
}

func (p *Peer) PortStr() string {
	return strconv.Itoa(int(p.Port))
}

// Unmarshal parses peer IP addresses and ports from a buffer
func Unmarshal(peersBin []byte) ([]Peer, error) {
	const ipv4PeerSize = 6
	const ipv6PeerSize = 18

	if len(peersBin)%ipv4PeerSize != 0 && len(peersBin)%ipv6PeerSize != 0 {
		return nil, fmt.Errorf("Received malformed peers of length %d", len(peersBin))
	}

	isIPv6 := false
	if len(peersBin)%ipv6PeerSize == 0 && len(peersBin)%ipv4PeerSize == 0 {
		// Ambiguous length (e.g. 18, 36...).
		// Check if first IPv4 peer's port is non-zero and second one is zero.
		// If the "second" IPv4 peer (bytes 10-12) has a port of 0, it's likely IPv6.
		if len(peersBin) >= ipv6PeerSize {
			port2 := binary.BigEndian.Uint16(peersBin[10:12])
			if port2 == 0 {
				isIPv6 = true
			}
		}
	} else if len(peersBin)%ipv6PeerSize == 0 {
		isIPv6 = true
	}

	peerSize := ipv4PeerSize
	if isIPv6 {
		peerSize = ipv6PeerSize
	}

	numPeers := len(peersBin) / peerSize

	peers := make([]Peer, numPeers)
	for i := 0; i < numPeers; i++ {
		offset := i * peerSize
		if isIPv6 {
			peers[i].IP = net.IP(peersBin[offset : offset+16]).String()
			peers[i].Port = binary.BigEndian.Uint16(peersBin[offset+16 : offset+18])
		} else {
			peers[i].IP = net.IP(peersBin[offset : offset+4]).String()
			peers[i].Port = binary.BigEndian.Uint16(peersBin[offset+4 : offset+6])
		}
	}
	return peers, nil
}
