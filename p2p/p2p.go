package p2p

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"github.com/pouyasadri/go-bittorrent/client"
	"github.com/pouyasadri/go-bittorrent/message"
	"github.com/pouyasadri/go-bittorrent/peers"
	"log"
	"runtime"
	"time"
)

// MaxBlockSize is the maximum size of a block
const MaxBlockSize = 16384

// MaxBacklog is the maximum number of blocks we will queue up
const MaxBacklog = 5

// Torrent holds data required to download a torrent from a list of peers
type Torrent struct {
	Peers       []peers.Peer
	PeerID      [20]byte
	InfoHash    [20]byte
	PieceHashes [][20]byte
	PieceLength int
	Length      int
	Name        string
}

type pieceWork struct {
	index  int
	hash   [20]byte
	length int
}

type pieceResult struct {
	index int
	buf   []byte
}

type pieceProgress struct {
	index      int
	client     *client.Client
	buf        []byte
	downloaded int
	requested  int
	backlog    int
}

func (state *pieceProgress) readMessage() error {
	msg, err := state.client.Read()
	if err != nil {
		return err
	}
	if msg == nil {
		return nil
	}
	switch msg.ID {
	case message.MsgUnchoke:
		state.client.Chocked = false
	case message.MsgChoke:
		state.client.Chocked = true
	case message.MsgHave:
		index, err := message.ParseHave(msg)
		if err != nil {
			return err
		}
		state.client.Bittfield.SetPiece(index)
	case message.MsgPiece:
		n, err := message.ParsePiece(state.index, state.buf, msg)
		if err != nil {
			return err
		}
		state.downloaded += n
		state.backlog--
	}
	return nil
}

func attemptDownloadPiece(c *client.Client, pw *pieceWork) ([]byte, error) {
	state := pieceProgress{
		index:  pw.index,
		client: c,
		buf:    make([]byte, pw.length),
	}
	// Setting a deadline helps get unresponsive peers unstuck.
	// 30 seconds is more than enough time to download a 262 KB piece.
	c.Conn.SetDeadline(time.Now().Add(30 * time.Second))
	defer c.Conn.SetDeadline(time.Time{})

	for state.downloaded < pw.length {
		// if unchoncked send request until we have enough unfufilled requests
		if !state.client.Chocked {
			for state.backlog < MaxBacklog && state.requested < pw.length {
				blockSize := MaxBlockSize
				// Last block might be shorter than the typical block.
				if pw.length-state.requested < MaxBlockSize {
					blockSize = pw.length - state.requested
				}
				err := c.SendRequest(pw.index, state.requested, blockSize)
				if err != nil {
					return nil, err
				}
				state.requested += blockSize
				state.backlog++
			}
		}
		err := state.readMessage()
		if err != nil {
			return nil, err
		}

	}
	return state.buf, nil
}

func checkIntegrity(pw *pieceWork, buf []byte) error {
	hash := sha1.Sum(buf)
	if !bytes.Equal(hash[:], pw.hash[:]) {
		return fmt.Errorf("index %d failed integrity check", pw.index)
	}
	return nil
}

func (t *Torrent) startDownloadWorker(peer peers.Peer, workQueue chan *pieceWork, result chan *pieceResult) {
	c, err := client.New(peer, t.PeerID, t.InfoHash)
	if err != nil {
		log.Printf("Clould not handshake with %s: %s", peer.String(), err)
		return
	}
	defer c.Conn.Close()

	log.Printf("Completed handshake with %s", peer.String())
	c.SendUnchoke()
	c.SendInterested()

	for pw := range workQueue {
		if !c.Bittfield.HasPiece(pw.index) {
			workQueue <- pw // put piece back on the queue
			continue
		}
		// download piece
		buf, err := attemptDownloadPiece(c, pw)
		if err != nil {
			log.Printf("exiting: %s", err)
			workQueue <- pw // put piece back on the queue
			return
		}

		err = checkIntegrity(pw, buf)
		if err != nil {
			log.Printf("Piece #%d failed integrity check: %s", pw.index, err)
			workQueue <- pw // put piece back on the queue
			continue
		}

		c.SendHave(pw.index)
		result <- &pieceResult{pw.index, buf}
	}
}

func (t *Torrent) calculateBoundsForPiece(index int) (begin int, end int) {
	begin = index * t.PieceLength
	end = begin + t.PieceLength
	if end >= t.Length {
		end = t.Length
	}
	return begin, end
}

func (t *Torrent) calculatePieceSize(index int) int {
	begin, end := t.calculateBoundsForPiece(index)
	return end - begin
}

// Download downloads the torrent . this stores the entire file in memory
func (t *Torrent) Download() ([]byte, error) {
	log.Println("Starting download for", t.Name)
	// Init queues for workers to retrieve work and send results
	workQueue := make(chan *pieceWork, len(t.PieceHashes))
	resultQueue := make(chan *pieceResult)
	for index, hash := range t.PieceHashes {
		length := t.calculatePieceSize(index)
		workQueue <- &pieceWork{index, hash, length}
	}

	// Start workers
	for _, peer := range t.Peers {
		go t.startDownloadWorker(peer, workQueue, resultQueue)
	}

	// Collect results into a buffer until full
	buf := make([]byte, t.Length)
	donePieces := 0
	for donePieces < len(t.PieceHashes) {
		res := <-resultQueue
		begin, end := t.calculateBoundsForPiece(res.index)
		copy(buf[begin:end], res.buf)
		donePieces++

		percent := float64(donePieces) / float64(len(t.PieceHashes)) * 100
		numWorkers := runtime.NumGoroutine() - 1 // subtract main thread

		log.Printf("(%0.2f%%) Downloaded piece #%d from %d peers", percent, res.index, numWorkers)

	}
	close(workQueue)
	return buf, nil
}
