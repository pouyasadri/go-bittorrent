package main

import (
	"github.com/pouyasadri/go-bittorrent/torrentfile"
	"log"
	"os"
)

func main() {
	inPath := os.Args[1]
	outPath := os.Args[2]

	tf, err := torrentfile.Open(inPath)
	if err != nil {
		log.Fatal(err)
	}
	err = tf.DownloadToFile(outPath)
	if err != nil {
		log.Fatal(err)
	}
}
