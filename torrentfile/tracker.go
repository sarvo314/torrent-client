package torrentfile

import (
	"crypto/rand"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"net/http"
	"net/url"
	"strconv"
	"strings"
	"time"

	"github.com/jackpal/bencode-go"
	"github.com/sarvo314/torrent-client/peers"
)

type bencodeTrackerResp struct {
	Interval      int          `bencode:"interval"`
	Peers         []peers.Peer `bencode:"peers"`
	FailureReason string       `bencode:"failure reason"`
}

func (t *TorrentFile) buildTrackerURL(peerID [20]byte, port uint16) (string, error) {
	base, err := url.Parse(t.Announce)
	if err != nil {
		return "", err
	}
	params := url.Values{
		"info_hash":  []string{string(t.InfoHash[:])},
		"peer_id":    []string{string(peerID[:])},
		"port":       []string{strconv.Itoa(int(port))},
		"uploaded":   []string{"0"},
		"downloaded": []string{"0"},
		"compact":    []string{"0"},
		"left":       []string{strconv.Itoa(t.Length)},
	}
	base.RawQuery = params.Encode()
	return base.String(), nil
}

func (t *TorrentFile) udpTrackerRequest(urlStr string, peerID [20]byte, port uint16) ([]peers.Peer, error) {
	u, err := url.Parse(urlStr)
	if err != nil {
		return nil, err
	}
	// host := u.Host
	// if !strings.Contains(host, ":") {
	// 	host = host + ":1337"
	// }
	conn, err := net.DialTimeout("udp", u.Host, 15*time.Second)

	if err != nil {
		return nil, fmt.Errorf("tracker unreachable: %w", err)
	}
	defer conn.Close()

	// Connect Phase
	// 32 bit transaction id for making connection
	var transactionID uint32
	err = binary.Read(rand.Reader, binary.BigEndian, &transactionID)
	if err != nil {
		return nil, err
	}

	// Send Connect Request
	// 64-bit connection_id: 0x41727101980 (magic constant)
	// 32-bit action: 0 (connect)
	// 32-bit transaction_id
	buf := make([]byte, 16)
	binary.BigEndian.PutUint64(buf[0:8], 0x41727101980) // 0x41727101980 is the magic constant
	binary.BigEndian.PutUint32(buf[8:12], 0)
	binary.BigEndian.PutUint32(buf[12:16], transactionID)

	conn.SetDeadline(time.Now().Add(15 * time.Second))
	_, err = conn.Write(buf)
	if err != nil {
		return nil, err
	}

	// Read Connect Response
	// 32-bit action: 0 (connect)
	// 32-bit transaction_id
	// 64-bit connection_id
	resp := make([]byte, 16)
	_, err = conn.Read(resp)
	if err != nil {
		return nil, err
	}

	action := binary.BigEndian.Uint32(resp[0:4])
	resTransactionID := binary.BigEndian.Uint32(resp[4:8])
	connectionID := binary.BigEndian.Uint64(resp[8:16]) // connectionID to be used in announce

	if action != 0 || resTransactionID != transactionID {
		return nil, fmt.Errorf("invalid connect response")
	}

	// TODO: Announce Phase
	// return nil, fmt.Errorf("announce phase not implemented yet")

	// Announce Phase
	var announceTransactionID uint32
	err = binary.Read(rand.Reader, binary.BigEndian, &announceTransactionID)

	if err != nil {
		return nil, err
	}
	log.Println("Now Announcing")
	buf = make([]byte, 98)
	var key uint32
	binary.Read(rand.Reader, binary.BigEndian, &key)

	binary.BigEndian.PutUint64(buf[0:8], connectionID)
	binary.BigEndian.PutUint32(buf[8:12], 1) // action 1: announce
	binary.BigEndian.PutUint32(buf[12:16], announceTransactionID)
	copy(buf[16:36], t.InfoHash[:])
	copy(buf[36:56], peerID[:])
	binary.BigEndian.PutUint64(buf[56:64], 0)                // downloaded
	binary.BigEndian.PutUint64(buf[64:72], uint64(t.Length)) // left
	binary.BigEndian.PutUint64(buf[72:80], 0)                // uploaded
	binary.BigEndian.PutUint32(buf[80:84], 0)                // event 0: none
	binary.BigEndian.PutUint32(buf[84:88], 0)                // IP 0: default (autodetect from my udp padcket)
	binary.BigEndian.PutUint32(buf[88:92], key)              // key: random
	binary.BigEndian.PutUint32(buf[92:96], 0xFFFFFFFF)       // num_want: -1 (default)
	binary.BigEndian.PutUint16(buf[96:98], port)             // port

	conn.SetDeadline(time.Now().Add(15 * time.Second))
	_, err = conn.Write(buf)

	if err != nil {
		return nil, err
	}
	resp = make([]byte, 1500)

	n, err := conn.Read(resp)
	resp = resp[:n]
	if err != nil {
		return nil, err
	}

	if n < 20 {
		return nil, fmt.Errorf("invalid announce response")
	}
	isIPv6 := false

	if (n-20)%6 == 0 {
		// IPv4
		isIPv6 = false
	} else if (n-20)%18 == 0 {
		// IPv6
		isIPv6 = true
	} else {
		return nil, fmt.Errorf("invalid announce response")
	}

	peerList := make([]peers.Peer, 0)

	if isIPv6 {
		for i := 20; i < n; i += 18 {
			ip := net.IP(resp[i : i+16])
			port := binary.BigEndian.Uint16(resp[i+16 : i+18])
			peerList = append(peerList, peers.Peer{
				IP:   ip.String(),
				Port: port,
			})
		}
	} else {
		for i := 20; i < n; i += 6 {
			ip := net.IP(resp[i : i+4])
			port := binary.BigEndian.Uint16(resp[i+4 : i+6])
			peerList = append(peerList, peers.Peer{
				IP:   ip.String(),
				Port: port,
			})
		}
	}

	log.Println("announce response: ", resp)
	log.Println("peerList: ", peerList)
	return peerList, nil
}

func (t *TorrentFile) requestPeers(peerID [20]byte, port uint16) ([]peers.Peer, error) {
	var lastErr error

	var peers []peers.Peer
	for _, tracker := range t.AnnounceList {
		isUDP := strings.HasPrefix(tracker, "udp://")

		var err error

		if isUDP {
			peer, err := t.udpTrackerRequest(tracker, peerID, port)
			if err != nil {
				log.Println("Error in udpTrackerRequest: ", err)
				continue
			}
			peers = append(peers, peer...)
		} else {
			url, e := t.buildTrackerURL(peerID, port)
			if e != nil {
				lastErr = e
				continue
			}
			// peers, err = tcpTrackerRequest(url)
			peer, err := tcpTrackerRequest(url)
			if err != nil {
				log.Println("Error in tcpTrackerRequest: ", err)
				continue
			}
			peers = append(peers, peer...)
		}

		// if err == nil && len(peers) > 0 {
		// 	return peers, nil
		// }

		lastErr = err
	}

	return nil, fmt.Errorf("all trackers failed: %w", lastErr)
}

// func (t *TorrentFile) requestPeers(peerID [20]byte, port uint16) ([]peers.Peer, error) {
// 	url, err := t.buildTrackerURL(peerID, port)
// 	if err != nil {
// 		return nil, err
// 	}
// 	isUDP := strings.HasPrefix(t.Announce, "udp://")
// 	if isUDP {
// 		return t.udpTrackerRequest(url, peerID, port)
// 		// return t.udpTrackerRequest("udp://tracker.opentrackr.org:1337/announce", peerID, port) //test url
// 	}
// 	//else is of type http
// 	return tcpTrackerRequest(url)
// }

func tcpTrackerRequest(url string) ([]peers.Peer, error) {
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	trackerResp := bencodeTrackerResp{}
	err = bencode.Unmarshal(resp.Body, &trackerResp)

	for _, peer := range trackerResp.Peers {
		fmt.Println(peer)
	}

	if err != nil {
		return nil, err
	}

	if trackerResp.FailureReason != "" {
		return nil, fmt.Errorf("tracker error: %s", trackerResp.FailureReason)
	}

	return trackerResp.Peers, nil
}
