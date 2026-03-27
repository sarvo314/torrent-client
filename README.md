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
- Minimal and readable implementation

## Improvements Over Original

Compared to the original project, this fork adds:

- **UDP tracker support**
  - Enables communication with a wider range of trackers
  - Implements UDP announce protocol

- **Multi-file torrent support**
  - Handles directory-based torrents
  - Correct file layout and piece mapping

## Advanced Enhancements

### Direct-to-Disk Downloading & Real-Time Progress

The download pipeline has been redesigned for efficiency and frontend integration.

- **Direct-to-Disk Piece Writing**  
  Eliminated large in-memory buffers. Pieces are written directly to disk as they arrive.

- **Multi-File Boundary Mapping**  
  Designed a `fileinfo` module to treat torrents as a continuous byte stream using
  `GlobalStart` and `GlobalEnd` offsets.

- **Real-Time Progress via WebSockets**  
  Implemented an `OnProgress` callback to stream live download progress to clients.

- **Robust State Management**  
  Safe handling of concurrent states (`Error`, `Downloading`, `Complete`) with consistent API responses.

## Technical Highlights

- Implemented UDP tracker protocol (connection + announce flow)
- Designed multi-file torrent support with continuous byte stream abstraction
- Built direct-to-disk piece writing pipeline (eliminated large memory buffers)
- Developed file boundary mapping using global offsets
- Implemented real-time progress streaming via WebSockets
- Designed concurrent download workers with safe state management
- Efficient piece-to-file boundary handling across multiple files
- Concurrent peer communication using goroutines

## Installation

```sh
go install github.com/<your-username>/torrent-client@latest
