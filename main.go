package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/gorilla/websocket"
	"github.com/sarvo314/torrent-client/torrentfile"
)

type DownloadState struct {
	Progress float64 `json:"progress"`
	Status   string  `json:"status"`
	Error    string  `json:"error,omitempty"`
}

var (
	downloads  = make(map[string]*DownloadState)
	stateMutex sync.Mutex
)
var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true }, // allow all origins
}

type DownloadRequest struct {
	TorrentPath string `json:"torrentPath"`
	OutPath     string `json:"outPath"`
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil) // upgrades HTTP → WebSocket
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()
	// Send progress updates in a loop
	for {
		stateMutex.Lock()
		snapshot := make(map[string]*DownloadState)
		for k, v := range downloads {
			snapshot[k] = v
		}
		stateMutex.Unlock()
		err := conn.WriteJSON(snapshot) // push state as JSON
		if err != nil {
			log.Println("WebSocket write error:", err)
			break
		}
		time.Sleep(1 * time.Second) // send update every second
	}
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

	errChan := make(chan error, 1)
	go startDownloadWorker(req.TorrentPath, req.OutPath, errChan)

	// Wait for the initialization result
	if err := <-errChan; err != nil {
		http.Error(w, fmt.Sprintf("Download failed: %v", err), http.StatusBadRequest)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusAccepted)
	json.NewEncoder(w).Encode(map[string]string{
		"message": "Download started",
	})
}

func startDownloadWorker(InPath, OutPath string, errChan chan<- error) {
	tf, err := torrentfile.Open(InPath)
	if err != nil {
		log.Println("Error opening torrent:", err)
		errChan <- err
		return
	}

	hashStr := hex.EncodeToString(tf.InfoHash[:])
	log.Println("Torrent hash:", hashStr)
	stateMutex.Lock()
	if _, ok := downloads[hashStr]; ok {
		if downloads[hashStr].Status == "Error" {
			delete(downloads, hashStr)
		} else {
			stateMutex.Unlock()
			log.Println("Download already in progress")
			errChan <- fmt.Errorf("download already in progress")
			return
		}
	}
	downloads[hashStr] = &DownloadState{
		Progress: 0,
		Status:   "Downloading",
	}
	stateMutex.Unlock()

	// Signal success — handler can now respond to the client
	errChan <- nil
	log.Println("Download started")

	err = tf.DownloadToFile(OutPath, func(percent float64) {
		stateMutex.Lock()
		defer stateMutex.Unlock()
		downloads[hashStr].Progress = percent
	})
	stateMutex.Lock()
	defer stateMutex.Unlock()
	if err != nil {
		log.Println("Error downloading:", err)
		downloads[hashStr].Status = "Error"
		downloads[hashStr].Error = err.Error()
		return
	}
	log.Println("Download complete!")
	downloads[hashStr].Status = "Complete"
	downloads[hashStr].Progress = 100
}

func main() {

	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/ws", wsHandler)

	log.Println("Server started on :8080")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
