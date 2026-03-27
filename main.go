package main

import (
	"encoding/hex"
	"encoding/json"
	"fmt"
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
	downloads  = make(map[string]*DownloadState)
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

	errChan := make(chan error, 1)
	go startDownloadWorker(req.TorrentPath, req.OutPath, errChan)

	// Wait for the initialization result
	w.Header().Set("Content-Type", "application/json")
	if err := <-errChan; err != nil {
		http.Error(w, fmt.Sprintf("Download failed: %v", err), http.StatusBadRequest)
		json.NewEncoder(w).Encode(map[string]string{
			"message": "Download failed",
		})
		return
	}

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
		switch downloads[hashStr].Status {
		case "Error":
			delete(downloads, hashStr)
		case "Downloading":
			stateMutex.Unlock()
			errChan <- fmt.Errorf("download already in progress")
			return
		case "Complete":
			stateMutex.Unlock()
			errChan <- fmt.Errorf("download already completed")
			return
		default:
			stateMutex.Unlock()
			errChan <- fmt.Errorf("state not found")
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
		downloads[hashStr].Progress = percent
		stateMutex.Unlock()
		broadcaster.Notify()
	})
	stateMutex.Lock()
	if err != nil {
		log.Println("Error downloading:", err)
		downloads[hashStr].Status = "Error"
		downloads[hashStr].Error = err.Error()
		stateMutex.Unlock()
		broadcaster.Notify()
		return
	}
	log.Println("Download complete!")
	downloads[hashStr].Status = "Complete"
	downloads[hashStr].Progress = 100
	stateMutex.Unlock()
	broadcaster.Notify()
}

func main() {
	go broadcaster.Run()

	http.HandleFunc("/download", downloadHandler)
	http.HandleFunc("/ws", wsHandler)

	log.Println("Server started on :8080")

	err := http.ListenAndServe(":8080", nil)
	if err != nil {
		log.Fatal(err)
	}
}
