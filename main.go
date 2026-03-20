package main

import (
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"sync"

	"github.com/sarvo314/torrent-client/torrentfile"
)

type DownloadState struct {
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	Error    string  `json:"error,omitempty"`
}

var (
	downloads  map[string]*DownloadState
	stateMutex sync.Mutex
)

type DownloadRequest struct {
	TorrentPath string `json:"torrentPath"`
	OutPath     string `json:"outPath"`
}

func downloadHandler(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req DownloadRequest
	err := json.NewDecoder(r.Body).Decode(&req)
	if err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return
	}
	downloads = make(map[string]*DownloadState)
	go startDownloadWorker(req.TorrentPath, req.OutPath)

	w.WriteHeader(http.StatusAccepted)
	w.Write([]byte(`{"message": "Download started"}`))

}

func startDownloadWorker(InPath, OutPath string) {
	tf, err := torrentfile.Open(InPath)

	if err != nil {
		log.Println("Error opening torrent:", err)
		return
	}
	hashStr := hex.EncodeToString(tf.InfoHash[:])
	log.Println("Torrent hash:", hashStr)
	stateMutex.Lock()
	defer stateMutex.Unlock()
	if _, ok := downloads[hashStr]; ok {
		stateMutex.Unlock()
		log.Println("Download already in progress")
		return
	}
	downloads[hashStr] = &DownloadState{
		Progress: 0,
		Status:   "Downloading",
	}
	stateMutex.Unlock()
	log.Println("Download started")

	err = tf.DownloadToFile(OutPath)
	if err != nil {
		log.Println("Error downloading:", err)
		return
	}
	log.Println("Download complete!")
}

func main() {

	http.HandleFunc("/download", downloadHandler)
	log.Println("Server started on :8080")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
