package main

import "sync"

// Broadcaster manages WebSocket clients and fans out state updates
type Broadcaster struct {
	clients    map[chan map[string]DownloadState]bool
	register   chan chan map[string]DownloadState
	unregister chan chan map[string]DownloadState
	broadcast  chan struct{}
	mu         sync.Mutex
}

func newBroadcaster() *Broadcaster {
	return &Broadcaster{
		clients:    make(map[chan map[string]DownloadState]bool),
		register:   make(chan chan map[string]DownloadState),
		unregister: make(chan chan map[string]DownloadState),
		broadcast:  make(chan struct{}, 1),
	}
}

func (b *Broadcaster) Run() {
	for {
		select {
		case client := <-b.register:
			b.mu.Lock()
			b.clients[client] = true
			b.mu.Unlock()
		case client := <-b.unregister:
			b.mu.Lock()
			delete(b.clients, client)
			b.mu.Unlock()
		case <-b.broadcast:
			stateMutex.Lock()
			snapshot := make(map[string]DownloadState)
			for k, v := range downloads {
				snapshot[k] = *v
			}
			stateMutex.Unlock()
			for client := range b.clients {
				select {
				case client <- snapshot:
				default:
				}
			}

		}

	}
}

func (b *Broadcaster) Notify() {
	select {
	case b.broadcast <- struct{}{}:
	default:
	}
}

var broadcaster = newBroadcaster()
