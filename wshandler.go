package main

import (
	"log"
	"net/http"

	"github.com/gorilla/websocket"
)

var upgrader = websocket.Upgrader{
	CheckOrigin: func(r *http.Request) bool { return true },
}

func wsHandler(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Println("WebSocket upgrade error:", err)
		return
	}
	defer conn.Close()

	updates := make(chan map[string]DownloadState, 10)
	broadcaster.register <- updates
	defer func() { broadcaster.unregister <- updates }()

	stateMutex.Lock()
	initial := make(map[string]DownloadState)
	for k, v := range downloads {
		initial[k] = *v
	}
	stateMutex.Unlock()
	conn.WriteJSON(initial)
	for snapshot := range updates {
		if err := conn.WriteJSON(snapshot); err != nil {
			log.Println("WebSocket write error:", err)
			break
		}
	}

}
