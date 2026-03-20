package torrentfile

import (
	"bytes"
	"crypto/rand"
	"crypto/sha1"
	"fmt"
	"log"
	"os"

	"path/filepath"

	"github.com/jackpal/bencode-go"
	"github.com/sarvo314/torrent-client/p2p"
)

// Port to listen on
const Port uint16 = 6881

// TorrentFile encodes the metadata from a .torrent file
type TorrentFile struct {
	Announce     string
	AnnounceList []string
	InfoHash     [20]byte
	PieceHashes  [][20]byte
	PieceLength  int
	Length       int
	Name         string
	Files        []File // Added for multi-file
}

type File struct {
	Length int
	Path   []string
}

// Remove bencodeInfo and bencodeTorrent structs and use map[string]interface{} instead

func flattenAnnounceList(al [][]string) []string {
	var result []string
	for _, tier := range al {
		for _, tracker := range tier {
			result = append(result, tracker)
		}
	}
	return result
}

// DownloadToFile downloads a torrent and writes it to a file
func (t *TorrentFile) DownloadToFile(path string) error {
	var peerID [20]byte
	_, err := rand.Read(peerID[:])
	if err != nil {
		return err
	}

	peers, err := t.requestPeers(peerID, Port)
	if err != nil {
		return err
	}

	torrent := p2p.Torrent{
		Peers:       peers,
		PeerID:      peerID,
		InfoHash:    t.InfoHash,
		PieceHashes: t.PieceHashes,
		PieceLength: t.PieceLength,
		Length:      t.Length,
		Name:        t.Name,
	}
	buf, err := torrent.Download()
	if err != nil {
		return err
	}

	if len(t.Files) == 0 {
		// Single file torrent
		outFile, err := os.Create(path)
		if err != nil {
			return err
		}
		defer outFile.Close()
		_, err = outFile.Write(buf)
		return err
	}

	// Multi-file torrent
	offset := 0
	for _, file := range t.Files {
		// Join path elements
		relPath := filepath.Join(file.Path...)
		fullPath := filepath.Join(path, relPath)

		// Create directories
		err := os.MkdirAll(filepath.Dir(fullPath), os.ModePerm)
		if err != nil {
			return err
		}

		// Write file
		err = os.WriteFile(fullPath, buf[offset:offset+file.Length], 0644)
		if err != nil {
			return err
		}
		offset += file.Length
	}

	return nil
}

// Open parses a torrent file
func Open(path string) (TorrentFile, error) {
	fileBytes, err := os.ReadFile(path)
	if err != nil {
		return TorrentFile{}, err
	}

	// 1. Calculate InfoHash from the raw bytes to be bit-perfect
	infoHash, err := extractInfoHash(fileBytes)
	if err != nil {
		return TorrentFile{}, fmt.Errorf("could not extract info hash: %w", err)
	}

	// 2. Unmarshal for other fields
	bto := bencodeTorrent{}
	err = bencode.Unmarshal(bytes.NewReader(fileBytes), &bto)
	if err != nil {
		return TorrentFile{}, fmt.Errorf("bencode unmarshal error: %w", err)
	}

	tf, err := bto.toTorrentFile()
	if err != nil {
		return TorrentFile{}, err
	}
	tf.InfoHash = infoHash // Override with bit-perfect hash

	// 3. Handle multi-file total length if needed (Length 0 means multi-file)
	if tf.Length == 0 {
		var total int64
		for _, f := range bto.Info.Files {
			total += int64(f.Length)
		}
		tf.Length = int(total)
	}

	return tf, nil
}

func extractInfoHash(data []byte) ([20]byte, error) {
	// Search for the 4:info key
	infoKey := []byte("4:info")
	index := bytes.Index(data, infoKey)
	if index == -1 {
		return [20]byte{}, fmt.Errorf("info key not found")
	}

	// The info dictionary starts right after "4:info"
	// and starts with 'd'
	infoStart := index + len(infoKey)
	if infoStart >= len(data) || data[infoStart] != 'd' {
		return [20]byte{}, fmt.Errorf("invalid info dictionary start")
	}

	// We need to find the matching 'e' for the info dictionary.
	// Bencode has 4 types: i (integer), l (list), d (dict), and <len>:<content> (string).
	// l and d are terminated by 'e'.
	// This is a simple stack-based parser to find the end of the dictionary at infoStart.

	end := findBencodeEnd(data[infoStart:])
	if end == -1 {
		return [20]byte{}, fmt.Errorf("could not find end of info dictionary")
	}

	rawInfo := data[infoStart : infoStart+end]
	return sha1.Sum(rawInfo), nil
}

func findBencodeEnd(data []byte) int {
	if len(data) == 0 {
		return -1
	}

	// This is a mini bencode parser to find the end of the root object
	pos := 0
	stack := 0

	for pos < len(data) {
		switch data[pos] {
		case 'd', 'l':
			stack++
			pos++
		case 'i':
			// skip until 'e'
			pos++
			for pos < len(data) && data[pos] != 'e' {
				pos++
			}
			if pos >= len(data) {
				return -1
			}
			pos++ // skip 'e'
		case 'e':
			stack--
			pos++
			if stack == 0 {
				return pos
			}
		case '0', '1', '2', '3', '4', '5', '6', '7', '8', '9':
			// string: <len>:<content>
			start := pos
			for pos < len(data) && data[pos] != ':' {
				pos++
			}
			if pos >= len(data) {
				return -1
			}
			var length int
			fmt.Sscanf(string(data[start:pos]), "%d", &length)
			pos++ // skip ':'
			pos += length
		default:
			return -1
		}
	}
	return -1
}

func (bto *bencodeTorrent) toTorrentFile() (TorrentFile, error) {
	pieceHashes, err := bto.Info.splitPieceHashes()
	if err != nil {
		return TorrentFile{}, err
	}
	announceList := flattenAnnounceList(bto.AnnounceList)
	if len(announceList) == 0 && bto.Announce != "" {
		announceList = []string{bto.Announce}
	}
	log.Printf("Announce list is %v\n", announceList)

	files := make([]File, len(bto.Info.Files))
	for i, f := range bto.Info.Files {
		files[i] = File{Length: f.Length, Path: f.Path}
	}

	return TorrentFile{
		Announce:     bto.Announce,
		AnnounceList: announceList,
		PieceHashes:  pieceHashes,
		PieceLength:  bto.Info.PieceLength,
		Length:       bto.Info.Length,
		Name:         bto.Info.Name,
		Files:        files,
	}, nil
}

func (i *bencodeInfo) splitPieceHashes() ([][20]byte, error) {
	hashLen := 20
	buf := []byte(i.Pieces)
	if len(buf)%hashLen != 0 {
		return nil, fmt.Errorf("Received malformed pieces of length %d", len(buf))
	}
	numHashes := len(buf) / hashLen
	hashes := make([][20]byte, numHashes)
	for i := 0; i < numHashes; i++ {
		copy(hashes[i][:], buf[i*hashLen:(i+1)*hashLen])
	}
	return hashes, nil
}

type bencodeInfo struct {
	Pieces      string `bencode:"pieces"`
	PieceLength int    `bencode:"piece length"`
	Length      int    `bencode:"length"`
	Name        string `bencode:"name"`
	Files       []struct {
		Length int      `bencode:"length"`
		Path   []string `bencode:"path"`
	} `bencode:"files"`
}

type bencodeTorrent struct {
	Announce     string      `bencode:"announce"`
	AnnounceList [][]string  `bencode:"announce-list"`
	Info         bencodeInfo `bencode:"info"`
}
