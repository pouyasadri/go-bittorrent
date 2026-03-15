package ratelimit

import (
	"net"
	"sync"
	"time"
)

type TokenBucket struct {
	mu           sync.Mutex
	tokens       float64
	maxTokens    float64
	fillRate     float64
	lastFilledAt time.Time
}

func NewTokenBucket(bytesPerSec int) *TokenBucket {
	maxTokens := float64(bytesPerSec) * 2 // allow 2 seconds worth of burst
	if bytesPerSec <= 0 {
		maxTokens = 0 // unlimited
	}
	return &TokenBucket{
		tokens:       maxTokens,
		maxTokens:    maxTokens,
		fillRate:     float64(bytesPerSec),
		lastFilledAt: time.Now(),
	}
}

func (tb *TokenBucket) Take(tokens float64) float64 {
	tb.mu.Lock()
	defer tb.mu.Unlock()

	if tb.fillRate <= 0 {
		return tokens // unlimited
	}

	now := time.Now()
	elapsed := now.Sub(tb.lastFilledAt).Seconds()
	tb.tokens += elapsed * tb.fillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastFilledAt = now

	if tb.tokens >= 1.0 {
		take := tokens
		if tb.tokens < take {
			take = tb.tokens
		}
		tb.tokens -= take
		return take
	}

	return 0
}

func (tb *TokenBucket) Wait(n int) int {
	if tb.fillRate <= 0 {
		return n // unlimited
	}

	taken := 0
	for taken == 0 {
		t := tb.Take(float64(n))
		if t >= 1.0 {
			taken = int(t)
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	return taken
}

// RateLimitedConn wraps a net.Conn with a TokenBucket limit on reads
type RateLimitedConn struct {
	net.Conn
	bucket *TokenBucket
}

func NewRateLimitedConn(conn net.Conn, bucket *TokenBucket) *RateLimitedConn {
	return &RateLimitedConn{
		Conn:   conn,
		bucket: bucket,
	}
}

func (c *RateLimitedConn) Read(p []byte) (int, error) {
	if c.bucket == nil || c.bucket.fillRate <= 0 {
		return c.Conn.Read(p)
	}
	allowed := c.bucket.Wait(len(p))
	return c.Conn.Read(p[:allowed])
}
