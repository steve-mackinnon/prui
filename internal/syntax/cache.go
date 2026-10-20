package syntax

import (
	"context"
	"crypto/sha256"
	"strings"
	"sync"
)

const tokenCacheBytes = 8 << 20
const tokenCacheEntries = 128

type tokenCacheKey struct{}
type tokenIdentity struct {
	path    string
	source  [32]byte
	through int // Zero is a complete capture; prefixes never satisfy full reads.
}
type tokenCache struct {
	mu      sync.Mutex
	entries map[tokenIdentity]Lines
	bytes   int
}

// WithCache scopes optional successful highlighting reuse to one capture. It
// retains neither source bytes nor failures and never changes source validation.
// Derived child contexts share the cache; independent opens do not.
func WithCache(parent context.Context) context.Context {
	return context.WithValue(parent, tokenCacheKey{}, &tokenCache{})
}

func cloneLines(lines Lines) Lines {
	result := make(Lines, len(lines))
	for line, spans := range lines {
		result[line] = append([]Span(nil), spans...)
	}
	return result
}

func (c *tokenCache) capture(ctx context.Context, path string, source []byte, compute func(context.Context, string, []byte) (Lines, Status)) (Lines, Status) {
	return c.captureThrough(ctx, path, source, 0, compute)
}

func cloneThrough(lines Lines, through int) Lines {
	if through <= 0 {
		return cloneLines(lines)
	}
	result := make(Lines, min(len(lines), through))
	for line, spans := range lines {
		if line <= through {
			result[line] = append([]Span(nil), spans...)
		}
	}
	return result
}

func (c *tokenCache) captureThrough(ctx context.Context, path string, source []byte, through int, compute func(context.Context, string, []byte) (Lines, Status)) (Lines, Status) {
	if ctx.Err() != nil {
		return nil, contextStatus(ctx)
	}
	if len(source) > MaxBytes || len(path) > 4096 {
		return compute(ctx, path, source)
	}
	key := tokenIdentity{path, sha256.Sum256(source), through}
	c.mu.Lock()
	lines, ok := c.entries[key]
	if !ok && through > 0 {
		fullKey := key
		fullKey.through = 0
		lines, ok = c.entries[fullKey]
	}
	if ok {
		result := cloneThrough(lines, through)
		c.mu.Unlock()
		if ctx.Err() != nil {
			return nil, contextStatus(ctx)
		}
		return result, Ready
	}
	c.mu.Unlock()
	lines, status := compute(ctx, path, source)
	if status != Ready || ctx.Err() != nil {
		return lines, status
	}
	// Account conservatively for keys, map buckets, slices and span capacity.
	// Every successful lexer result already has the per-blob MaxSpans bound.
	cost := 1024 + len(path)
	for _, spans := range lines {
		cost += 256 + len(spans)*64
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, ok := c.entries[key]; !ok && len(c.entries) < tokenCacheEntries && cost <= tokenCacheBytes-c.bytes {
		if c.entries == nil {
			c.entries = map[tokenIdentity]Lines{}
		}
		key.path = strings.Clone(path)
		c.entries[key] = cloneLines(lines)
		c.bytes += cost
	}
	return lines, status
}
