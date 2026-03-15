package ratelimit

import (
	"bytes"
	"net"
	"testing"
	"time"
)

func TestTokenBucketTake(t *testing.T) {
	// 100 bytes/sec
	tb := NewTokenBucket(100)

	// Initially should have 200 tokens (maxTokens)
	if tb.tokens != 200 {
		t.Errorf("Expected 200 tokens initially, got %f", tb.tokens)
	}

	// Take 50 tokens
	taken := tb.Take(50)
	if taken != 50 {
		t.Errorf("Expected to take 50 tokens, got %f", taken)
	}

	if tb.tokens > 150 {
		t.Errorf("Expected tokens to be <= 150, got %f", tb.tokens)
	}

	// unlimited test
	unlimitedTb := NewTokenBucket(0)
	takenUnlimited := unlimitedTb.Take(50)
	if takenUnlimited != 50 {
		t.Errorf("Expected to take 50 tokens from unlimited bucket, got %f", takenUnlimited)
	}
}

func TestTokenBucketWait(t *testing.T) {
	tb := NewTokenBucket(100)

	// Take all tokens
	tb.Take(200)

	start := time.Now()
	// Wait should block until at least 1 token is available, then return
	taken := tb.Wait(50)
	duration := time.Since(start)

	if taken <= 0 {
		t.Errorf("Expected to wait and take >0 tokens, got %d", taken)
	}

	// Should take ~10ms to refill at least 1 token at 100 bytes/sec
	if duration < 5*time.Millisecond {
		t.Errorf("Wait did not block long enough: %v", duration)
	}

	// unlimited test
	unlimitedTb := NewTokenBucket(0)
	startUnlimited := time.Now()
	takenUnlimited := unlimitedTb.Wait(50)
	durationUnlimited := time.Since(startUnlimited)
	if takenUnlimited != 50 {
		t.Errorf("Expected to wait and take 50 tokens from unlimited bucket, got %d", takenUnlimited)
	}
	if durationUnlimited > 100*time.Millisecond {
		t.Errorf("Wait on unlimited bucket blocked too long: %v", durationUnlimited)
	}
}

type mockConn struct {
	net.Conn
	readBuf *bytes.Buffer
}

func (m *mockConn) Read(b []byte) (n int, err error) {
	return m.readBuf.Read(b)
}

func TestRateLimitedConn(t *testing.T) {
	data := []byte("hello world")
	conn := &mockConn{readBuf: bytes.NewBuffer(data)}

	tb := NewTokenBucket(5) // 5 bytes/sec
	rlc := NewRateLimitedConn(conn, tb)

	buf := make([]byte, 11)

	start := time.Now()
	// The first 10 bytes should be read immediately from the 10-byte burst capacity
	n, err := rlc.Read(buf[:10])
	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if n != 10 {
		t.Errorf("Expected to read 10 bytes, got %d", n)
	}

	// The next 1 byte should take ~0.2 seconds (fill rate 5 bytes/sec => 1 byte / 5 = 0.2s)
	n2, err2 := rlc.Read(buf[10:])
	duration := time.Since(start)
	if err2 != nil {
		t.Fatalf("Unexpected error: %v", err2)
	}
	if n2 != 1 {
		t.Errorf("Expected to read 1 byte, got %d", n2)
	}
	if duration < 200*time.Millisecond {
		t.Errorf("RateLimitedConn read too fast: %v", duration)
	}

	if string(buf) != "hello world" {
		t.Errorf("Expected 'hello world', got '%s'", string(buf))
	}
}

func TestRateLimitedConnUnlimited(t *testing.T) {
	data := []byte("hello world")
	conn := &mockConn{readBuf: bytes.NewBuffer(data)}

	tb := NewTokenBucket(0) // unlimited
	rlc := NewRateLimitedConn(conn, tb)

	buf := make([]byte, 11)
	start := time.Now()
	n, err := rlc.Read(buf)
	duration := time.Since(start)

	if err != nil {
		t.Fatalf("Unexpected error: %v", err)
	}
	if n != 11 {
		t.Errorf("Expected to read 11 bytes, got %d", n)
	}
	if duration > 10*time.Millisecond {
		t.Errorf("Unlimited Read blocked too long: %v", duration)
	}
}
