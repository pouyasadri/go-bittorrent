# Go-Bittorrent

Go-Bittorrent is a simple BitTorrent client implemented in Go. It allows you to download torrent files from the BitTorrent network.

## Features

- Open and parse torrent files: The client can read torrent files and extract the necessary information to start the download process.
- Download torrent files to a specified path: The client can download files from the BitTorrent network and save them to a specified location on your local system.

## Installation

To install Go-Bittorrent, you need to have Go installed on your system. You can download and install Go from [here](https://golang.org/dl/).

After installing Go, clone the repository:

```bash
git clone https://github.com/pouyasadri/go-bittorrent.git
```

Then, navigate to the project directory:

```bash
cd go-bittorrent
```
## Usage
To use Go-Bittorrent, you need to provide two arguments: the path to the torrent file and the output path for the downloaded file.

```bash
go run main.go <path-to-torrent-file> <output-path>
```
## Testing
Go-Bittorrent comes with a suite of tests to ensure its functionality. To run the tests:

```bash
go test ./...
```
## Contributing
Contributions to Go-Bittorrent are welcome. If you want to contribute, please open a pull request. For major changes, please open an issue first to discuss what you would like to change.
