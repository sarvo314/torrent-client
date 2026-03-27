# torrent-client (Enhanced Fork)

A lightweight BitTorrent client written in Go, originally based on
https://github.com/veggiedefender/torrent-client.

This fork extends the original implementation with additional protocol support
and improved functionality.

## Features

- Supports `.torrent` files
- HTTP tracker support
- UDP tracker support
- Multi-file torrent support
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
 
- **Write to Disk Directly**
  - Instead of storing buffer on RAM now it's stored on disk


## Installation

```sh
go install github.com/<your-username>/torrent-client@latest
