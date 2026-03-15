package p2p

import (
	"bytes"
	"crypto/sha1"
	"encoding/binary"
	"net"
	"testing"
	"time"

	"github.com/pouyasadri/go-bittorrent/bitfield"
	"github.com/pouyasadri/go-bittorrent/client"
	"github.com/pouyasadri/go-bittorrent/message"
	"github.com/pouyasadri/go-bittorrent/peers"
	"os"
)

func formatPiece(index, begin int, data []byte) message.Message {
	payload := make([]byte, 8+len(data))
	binary.BigEndian.PutUint32(payload[0:4], uint32(index))
	binary.BigEndian.PutUint32(payload[4:8], uint32(begin))
	copy(payload[8:], data)
	return message.Message{ID: message.MsgPiece, Payload: payload}
}

func TestCalculateBoundsForPiece(t *testing.T) {
	torrent := &Torrent{
		PieceLength: 10,
		Length:      25,
	}

	tests := []struct {
		index    int
		expBegin int
		expEnd   int
	}{
		{0, 0, 10},
		{1, 10, 20},
		{2, 20, 25},
	}

	for _, tt := range tests {
		b, e := torrent.calculateBoundsForPiece(tt.index)
		if b != tt.expBegin || e != tt.expEnd {
			t.Errorf("calculateBoundsForPiece(%d) = %d, %d; want %d, %d", tt.index, b, e, tt.expBegin, tt.expEnd)
		}
	}
}

func TestCalculatePieceSize(t *testing.T) {
	torrent := &Torrent{
		PieceLength: 10,
		Length:      25,
	}

	tests := []struct {
		index   int
		expSize int
	}{
		{0, 10},
		{1, 10},
		{2, 5},
	}

	for _, tt := range tests {
		size := torrent.calculatePieceSize(tt.index)
		if size != tt.expSize {
			t.Errorf("calculatePieceSize(%d) = %d; want %d", tt.index, size, tt.expSize)
		}
	}
}

func TestCheckIntegrity(t *testing.T) {
	buf := []byte("hello world")
	correctHash := sha1.Sum(buf)

	wrongHash := sha1.Sum([]byte("wrong"))

	pw := &pieceWork{
		index: 0,
		hash:  correctHash,
	}

	// Should pass
	err := checkIntegrity(pw, buf)
	if err != nil {
		t.Errorf("expected no error, got %v", err)
	}

	// Should fail
	pw2 := &pieceWork{
		index: 1,
		hash:  wrongHash,
	}
	err = checkIntegrity(pw2, buf)
	if err == nil {
		t.Errorf("expected error, got nil")
	}
}

type mockConn struct {
	net.Conn
	readBuf  *bytes.Buffer
	writeBuf *bytes.Buffer
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return m.readBuf.Read(b)
}
func (m *mockConn) Write(b []byte) (n int, err error) {
	return m.writeBuf.Write(b)
}
func (m *mockConn) SetDeadline(t time.Time) error {
	return nil
}

func TestReadMessage(t *testing.T) {
	c := &client.Client{
		Conn: &mockConn{
			readBuf:  bytes.NewBuffer(nil),
			writeBuf: bytes.NewBuffer(nil),
		},
		Choked:   true,
		Bitfield: make(bitfield.Bitfield, 1),
	}

	state := &pieceProgress{
		index:  0,
		client: c,
		buf:    make([]byte, 10),
	}

	// 1. Unchoke
	unchokeMsg := message.Message{ID: message.MsgUnchoke}
	c.Conn.(*mockConn).readBuf.Write(unchokeMsg.Serialize())
	err := state.readMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if c.Choked {
		t.Errorf("expected to be unchoked")
	}

	// 2. Choke
	chokeMsg := message.Message{ID: message.MsgChoke}
	c.Conn.(*mockConn).readBuf.Write(chokeMsg.Serialize())
	err = state.readMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Choked {
		t.Errorf("expected to be choked")
	}

	// 3. Have
	haveMsg := message.FormatHave(0)
	c.Conn.(*mockConn).readBuf.Write(haveMsg.Serialize())
	err = state.readMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !c.Bitfield.HasPiece(0) {
		t.Errorf("expected bitfield to have piece 0")
	}

	// 4. Piece
	pieceMsg := formatPiece(0, 0, []byte("hello"))
	c.Conn.(*mockConn).readBuf.Write(pieceMsg.Serialize())
	state.backlog = 1
	err = state.readMessage()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if state.downloaded != 5 {
		t.Errorf("expected downloaded to be 5, got %d", state.downloaded)
	}
	if state.backlog != 0 {
		t.Errorf("expected backlog to have decremented to 0")
	}
	if string(state.buf[:5]) != "hello" {
		t.Errorf("expected buffer to start with hello, got %s", string(state.buf[:5]))
	}
}

func TestAttemptDownloadPiece(t *testing.T) {
	c := &client.Client{
		Conn: &mockConn{
			readBuf:  bytes.NewBuffer(nil),
			writeBuf: bytes.NewBuffer(nil),
		},
		Choked:   false,
		Bitfield: make(bitfield.Bitfield, 1),
	}

	pw := &pieceWork{
		index:  0,
		length: 10,
	}

	// We put the piece response in the read buffer beforehand.
	// Since length is 10, it only expects 1 piece message response.
	pieceMsg := formatPiece(0, 0, bytes.Repeat([]byte("a"), 10))
	c.Conn.(*mockConn).readBuf.Write(pieceMsg.Serialize())

	buf, err := attemptDownloadPiece(c, pw)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(buf) != 10 || string(buf) != string(bytes.Repeat([]byte("a"), 10)) {
		t.Errorf("expected to download piece successfully, got %v", string(buf))
	}
}

func TestStartDownloadWorker(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	peerID := [20]byte{1, 1, 1}
	infoHash := [20]byte{2, 2, 2}

	pieceData := []byte("1234567890")
	pieceHash := sha1.Sum(pieceData)

	done := make(chan struct{})
	go func() {
		defer ln.Close()
		serverConn, err := ln.Accept()
		if err != nil {
			return
		}

		// 1. Read Handshake
		buf := make([]byte, 68)
		serverConn.Read(buf)

		// 2. Write Handshake Back
		serverHandshake := []byte{19, 66, 105, 116, 84, 111, 114, 114, 101, 110, 116, 32, 112, 114, 111, 116, 111, 99, 111, 108, 0, 0, 0, 0, 0, 0, 0, 0}
		serverHandshake = append(serverHandshake, infoHash[:]...)
		serverHandshake = append(serverHandshake, peerID[:]...)
		serverConn.Write(serverHandshake)

		// 3. Write Bitfield (simulate having piece 0)
		bitfieldMsg := []byte{0x00, 0x00, 0x00, 0x02, 5, 0x80} // 0x80 = 10000000 (piece 0)
		serverConn.Write(bitfieldMsg)

		// 4. Read Unchoke & Interested
		buf2 := make([]byte, 10)
		serverConn.Read(buf2) // simple read for request logic

		// 5. Provide Piece response
		// Just wait a tiny bit to make sure request is sent
		time.Sleep(10 * time.Millisecond)
		pieceMsg := formatPiece(0, 0, pieceData)
		serverConn.Write(pieceMsg.Serialize())

		close(done)
	}()

	addr := ln.Addr().(*net.TCPAddr)
	p := peers.Peer{
		IP:   addr.IP,
		Port: uint16(addr.Port),
	}

	workQueue := make(chan *pieceWork, 1)
	resultQueue := make(chan *pieceResult, 1)

	workQueue <- &pieceWork{
		index:  0,
		hash:   pieceHash,
		length: 10,
	}
	close(workQueue)

	tor := &Torrent{
		PeerID:   peerID,
		InfoHash: infoHash,
		Bucket:   nil,
	}

	tor.startDownloadWorker(p, workQueue, resultQueue)

	// verify results
	res := <-resultQueue
	if res.index != 0 {
		t.Errorf("Expected index 0, got %v", res.index)
	}
	if string(res.buf) != string(pieceData) {
		t.Errorf("Expected piece data, got %v", res.buf)
	}

	<-done
}

func TestDownload(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("failed to listen: %v", err)
	}

	peerID := [20]byte{1, 1, 1}
	infoHash := [20]byte{2, 2, 2}

	pieceData := []byte("1234567890")
	pieceHash := sha1.Sum(pieceData)

	go func() {
		defer ln.Close()
		serverConn, err := ln.Accept()
		if err != nil {
			return
		}

		buf := make([]byte, 68)
		serverConn.Read(buf)

		serverHandshake := []byte{19, 66, 105, 116, 84, 111, 114, 114, 101, 110, 116, 32, 112, 114, 111, 116, 111, 99, 111, 108, 0, 0, 0, 0, 0, 0, 0, 0}
		serverHandshake = append(serverHandshake, infoHash[:]...)
		serverHandshake = append(serverHandshake, peerID[:]...)
		serverConn.Write(serverHandshake)

		bitfieldMsg := []byte{0x00, 0x00, 0x00, 0x02, 5, 0x80} // 0x80 = 10000000 (piece 0)
		serverConn.Write(bitfieldMsg)

		buf2 := make([]byte, 10)
		serverConn.Read(buf2)

		pieceMsg := formatPiece(0, 0, pieceData)
		serverConn.Write(pieceMsg.Serialize())
	}()

	addr := ln.Addr().(*net.TCPAddr)
	p := peers.Peer{
		IP:   addr.IP,
		Port: uint16(addr.Port),
	}

	tor := &Torrent{
		Peers:       []peers.Peer{p},
		PeerID:      peerID,
		InfoHash:    infoHash,
		PieceHashes: [][20]byte{pieceHash},
		PieceLength: 10,
		Length:      10,
		Name:        "test-download",
	}

	tmpFile, err := os.CreateTemp("", "test-download-p2p")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	defer tmpFile.Close()

	err = tor.Download(tmpFile)
	if err != nil {
		t.Fatalf("Download failed: %v", err)
	}

	// Verify temp file contents
	fileData, err := os.ReadFile(tmpFile.Name())
	if err != nil {
		t.Fatalf("failed to read temp file: %v", err)
	}
	if string(fileData) != string(pieceData) {
		t.Errorf("expected %v, got %v", pieceData, fileData)
	}
}
