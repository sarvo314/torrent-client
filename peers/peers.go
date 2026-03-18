package peers

import (
	"net"
	"strconv"
)

type Peer struct {
	PeerID string `bencode:"peer id"`
	IP     string `bencode:"ip"`
	Port   uint16 `bencode:"port"`
}

func (p *Peer) String() string {
	return net.JoinHostPort(p.IP, strconv.Itoa(int(p.Port)))
}
