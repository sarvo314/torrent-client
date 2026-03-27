# torrent-client (Enhanced Fork)

A lightweight BitTorrent client written in Go, originally based on  
https://github.com/veggiedefender/torrent-client.

This fork extends the original implementation with additional protocol support,
improved I/O design, and real-time streaming capabilities.

## Features

- Supports `.torrent` files
- HTTP tracker support
- UDP tracker support
- Multi-file torrent support
- Direct-to-disk downloading (low memory usage)
- Real-time progress updates via WebSockets
- Concurrent piece downloading

### Improvements Over Original

Compared to the original project, this fork adds:

- **UDP tracker support**
- **Multi-file torrent downloading**
- **Direct-to-disk piece writing (no large memory buffers)**
- **WebSocket-based real-time status/progress updates**


## Installation

```sh
go install github.com/<your-username>/torrent-client@latest
