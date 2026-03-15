package main

import (
	"flag"
	"fmt"
	"log"
	"os"

	"github.com/pterm/pterm"
	"github.com/pouyasadri/go-bittorrent/ratelimit"
	"github.com/pouyasadri/go-bittorrent/torrentfile"
)

func main() {
	outPath := flag.String("out", "", "Output path for the downloaded file")
	port := flag.Int("port", 6881, "Port to listen on for peer connections")
	maxDownload := flag.Int("max-download", 0, "Max download speed in KB/s (0 for unlimited)")
	debug := flag.Bool("debug", false, "Enable detailed debug logging")

	flag.Usage = func() {
		fmt.Fprintf(os.Stderr, "Usage: %s [options] <torrent-file>\n", os.Args[0])
		fmt.Fprintf(os.Stderr, "Options:\n")
		flag.PrintDefaults()
	}

	flag.Parse()

	if *outPath == "" {
		log.Fatal("Error: --out flag is required (e.g., --out=debian.iso)")
	}

	if flag.NArg() < 1 {
		log.Fatal("Error: path to torrent file is required")
	}

	inPath := flag.Arg(0)

	tf, err := torrentfile.Open(inPath)
	if err != nil {
		log.Fatal(err)
	}
	
	if *debug {
		pterm.EnableDebugMessages()
	} else {
		pterm.DisableDebugMessages()
	}

	// convert KB/s to Bytes/s
	bucket := ratelimit.NewTokenBucket(*maxDownload * 1024)

	err = tf.DownloadToFile(*outPath, uint16(*port), bucket)
	if err != nil {
		log.Fatal(err)
	}
}
