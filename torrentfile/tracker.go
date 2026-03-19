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
	Interval      int         `bencode:"interval"`
	Peers         interface{} `bencode:"peers"`
	FailureReason string      `bencode:"failure reason"`
}

func (t *TorrentFile) buildTrackerURL(tracker string, peerID [20]byte, port uint16) (string, error) {
	base, err := url.Parse(tracker)
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

	conn, err := net.DialTimeout("udp", u.Host, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("tracker unreachable: %w", err)
	}
	defer conn.Close()

	// Connect Phase
	var transactionID uint32
	binary.Read(rand.Reader, binary.BigEndian, &transactionID)

	connReq := make([]byte, 16)
	binary.BigEndian.PutUint64(connReq[0:8], 0x41727101980) // magic constant
	binary.BigEndian.PutUint32(connReq[8:12], 0)           // action 0: connect
	binary.BigEndian.PutUint32(connReq[12:16], transactionID)

	connResp, err := udpExchange(conn, connReq)
	if err != nil {
		return nil, fmt.Errorf("connect phase failed: %w", err)
	}

	if len(connResp) < 16 {
		return nil, fmt.Errorf("invalid connect response length")
	}
	action := binary.BigEndian.Uint32(connResp[0:4])
	resTransactionID := binary.BigEndian.Uint32(connResp[4:8])
	connectionID := binary.BigEndian.Uint64(connResp[8:16])

	if action != 0 || resTransactionID != transactionID {
		return nil, fmt.Errorf("invalid connect response (action %d, tid %d)", action, resTransactionID)
	}

	// Announce Phase
	var announceTransactionID uint32
	binary.Read(rand.Reader, binary.BigEndian, &announceTransactionID)

	announceReq := make([]byte, 98)
	var key uint32
	binary.Read(rand.Reader, binary.BigEndian, &key)

	binary.BigEndian.PutUint64(announceReq[0:8], connectionID)
	binary.BigEndian.PutUint32(announceReq[8:12], 1) // action 1: announce
	binary.BigEndian.PutUint32(announceReq[12:16], announceTransactionID)
	copy(announceReq[16:36], t.InfoHash[:])
	copy(announceReq[36:56], peerID[:])
	binary.BigEndian.PutUint64(announceReq[56:64], 0)                // downloaded
	binary.BigEndian.PutUint64(announceReq[64:72], uint64(t.Length)) // left
	binary.BigEndian.PutUint64(announceReq[72:80], 0)                // uploaded
	binary.BigEndian.PutUint32(announceReq[80:84], 0)                // event 0: none
	binary.BigEndian.PutUint32(announceReq[84:88], 0)                // IP 0: default
	binary.BigEndian.PutUint32(announceReq[88:92], key)              // key: random
	binary.BigEndian.PutUint32(announceReq[92:96], 0xFFFFFFFF)       // num_want: -1
	binary.BigEndian.PutUint16(announceReq[96:98], port)             // port

	announceResp, err := udpExchange(conn, announceReq)
	if err != nil {
		return nil, fmt.Errorf("announce phase failed: %w", err)
	}

	if len(announceResp) < 20 {
		return nil, fmt.Errorf("invalid announce response length")
	}

	action = binary.BigEndian.Uint32(announceResp[0:4])
	resTransactionID = binary.BigEndian.Uint32(announceResp[4:8])

	if action == 3 {
		return nil, fmt.Errorf("tracker error: %s", string(announceResp[8:]))
	}
	if action != 1 || resTransactionID != announceTransactionID {
		return nil, fmt.Errorf("invalid announce response (action %d, tid %d)", action, resTransactionID)
	}

	return peers.Unmarshal(announceResp[20:])
}

func udpExchange(conn net.Conn, request []byte) ([]byte, error) {
	var lastErr error
	maxRetries := 3

	for i := 0; i < maxRetries; i++ {
		// Set a shorter deadline for the initial attempt
		timeout := time.Duration(15*(1<<i)) * time.Second
		conn.SetDeadline(time.Now().Add(timeout))

		_, err := conn.Write(request)
		if err != nil {
			lastErr = err
			continue
		}

		response := make([]byte, 1500)
		n, err := conn.Read(response)
		if err == nil {
			return response[:n], nil
		}
		lastErr = err
		log.Printf("UDP request failed (attempt %d): %v. Retrying...\n", i+1, err)
	}
	return nil, lastErr
}

func (t *TorrentFile) requestPeers(peerID [20]byte, port uint16) ([]peers.Peer, error) {
	var allPeers []peers.Peer
	peerMap := make(map[string]bool)

	for _, tracker := range t.AnnounceList {
		var peer []peers.Peer
		var err error

		switch {
		case strings.HasPrefix(tracker, "udp://"):
			peer, err = t.udpTrackerRequest(tracker, peerID, port)
		case strings.HasPrefix(tracker, "http://"), strings.HasPrefix(tracker, "https://"):
			url, e := t.buildTrackerURL(tracker, peerID, port)
			if e != nil {
				log.Println("buildTrackerURL error:", e)
				continue
			}
			peer, err = tcpTrackerRequest(url)
		default:
			continue
		}

		if err != nil {
			log.Printf("Tracker %s error: %v\n", tracker, err)
			continue
		}
		// peer deduplication
		for _, p := range peer {
			addr := p.String()
			if !peerMap[addr] {
				allPeers = append(allPeers, p)
				peerMap[addr] = true
			}
		}
	}

	if len(allPeers) == 0 {
		return nil, fmt.Errorf("could not find any peers from any tracker")
	}

	return allPeers, nil
}

func tcpTrackerRequest(url string) ([]peers.Peer, error) {
	c := &http.Client{Timeout: 15 * time.Second}
	resp, err := c.Get(url)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	trackerResp := bencodeTrackerResp{}
	err = bencode.Unmarshal(resp.Body, &trackerResp)
	if err != nil {
		return nil, err
	}

	if trackerResp.FailureReason != "" {
		return nil, fmt.Errorf("tracker error: %s", trackerResp.FailureReason)
	}

	switch p := trackerResp.Peers.(type) {
	case string:
		log.Println("format is of string")
		return peers.Unmarshal([]byte(p))
	case []interface{}:
		log.Println("format is of list")
		return parsePeersFromList(p)
	default:
		return nil, fmt.Errorf("invalid peers format: %T", trackerResp.Peers)
	}
}

func parsePeersFromList(list []interface{}) ([]peers.Peer, error) {
	peerList := make([]peers.Peer, len(list))
	for i, item := range list {
		m, ok := item.(map[string]interface{})
		if !ok {
			return nil, fmt.Errorf("invalid peer format in list")
		}
		peerList[i] = peers.Peer{
			IP:   m["ip"].(string),
			Port: uint16(m["port"].(int64)),
		}
		if pid, ok := m["peer id"].(string); ok {
			peerList[i].PeerID = pid
		}
	}
	return peerList, nil
}
