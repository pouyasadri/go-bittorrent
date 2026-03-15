package p2p

import (
	"bytes"
	"crypto/sha1"
	"fmt"
	"os"
	"runtime"
	"time"

	"github.com/pterm/pterm"

	"github.com/pouyasadri/go-bittorrent/client"
	"github.com/pouyasadri/go-bittorrent/message"
	"github.com/pouyasadri/go-bittorrent/peers"
	"github.com/pouyasadri/go-bittorrent/ratelimit"
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
	Bucket      *ratelimit.TokenBucket
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
		state.client.Choked = false
	case message.MsgChoke:
		state.client.Choked = true
	case message.MsgHave:
		index, err := message.ParseHave(msg)
		if err != nil {
			return err
		}
		state.client.Bitfield.SetPiece(index)
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
		if !state.client.Choked {
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
	c, err := client.New(peer, t.PeerID, t.InfoHash, t.Bucket)
	if err != nil {
		pterm.Debug.Printf("Could not handshake with %s: %s\n", peer.String(), err)
		return
	}
	defer c.Conn.Close()

	pterm.Debug.Printf("Completed handshake with %s\n", peer.String())
	c.SendUnchoke()
	c.SendInterested()

	for pw := range workQueue {
		if !c.Bitfield.HasPiece(pw.index) {
			workQueue <- pw // put piece back on the queue
			continue
		}
		// download piece
		buf, err := attemptDownloadPiece(c, pw)
		if err != nil {
			pterm.Debug.Printf("exiting: %s\n", err)
			workQueue <- pw // put piece back on the queue
			return
		}

		err = checkIntegrity(pw, buf)
		if err != nil {
			pterm.Debug.Printf("Piece #%d failed integrity check: %s\n", pw.index, err)
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

// Download downloads the torrent directly to a file
func (t *Torrent) Download(outFile *os.File) error {
	pterm.Info.Printf("Starting download for %s\n", t.Name)
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

	bar, _ := pterm.DefaultProgressbar.WithTotal(len(t.PieceHashes)).WithTitle("Downloading " + t.Name).Start()

	// Collect results into a file until full
	donePieces := 0
	for donePieces < len(t.PieceHashes) {
		res := <-resultQueue
		begin, _ := t.calculateBoundsForPiece(res.index)
		_, err := outFile.WriteAt(res.buf, int64(begin))
		if err != nil {
			return err
		}
		donePieces++

		numWorkers := runtime.NumGoroutine() - 1 // subtract main thread
		
		if bar != nil {
		    bar.UpdateTitle(fmt.Sprintf("Downloading (Peers: %d)", numWorkers))
		    bar.Increment()
		}
	}
	
	if bar != nil {
	    bar.Stop()
	}
	
	close(workQueue)
	return nil
}
