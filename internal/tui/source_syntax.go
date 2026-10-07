package tui

import (
	"context"
	"time"

	"prui/internal/inventory"
	"prui/internal/review"
	"prui/internal/syntax"
)

type sourceSyntaxKey struct{ oid, path string }

// sourceSyntaxCache belongs to one tab and stores theme-independent results,
// including failed attempts, so projection invalidation never repeats lexing.
type sourceSyntaxCache struct {
	session      *review.Session
	entries      map[sourceSyntaxKey]syntax.Lines
	bytes, spans int
	elapsed      time.Duration
}

func (m *Model) sourceSyntaxCache() *sourceSyntaxCache {
	if m.sourceTokens == nil || m.sourceTokens.session != m.Session {
		m.sourceTokens = &sourceSyntaxCache{session: m.Session, entries: make(map[sourceSyntaxKey]syntax.Lines)}
	}
	return m.sourceTokens
}

func (c *sourceSyntaxCache) tokens(ctx context.Context, inv inventory.Inventory, file int, old bool) syntax.Lines {
	f := inv.Files[file]
	key := sourceSyntaxKey{f.NewOID, string(f.NewPath)}
	if old {
		key = sourceSyntaxKey{f.OldOID, string(f.OldPath)}
	}
	if inv.FullSource == nil {
		return nil
	}
	raw, ok := inv.FullSource.Blobs[key.oid]
	if !ok {
		return nil
	} // Missing source can arrive later; it is not a lexing failure.
	if result, ok := c.entries[key]; ok {
		return result
	}
	c.entries[key] = nil
	remaining := 500*time.Millisecond - c.elapsed
	if len(raw) > syntax.MaxBytes || c.bytes+len(raw) > 4<<20 || c.spans >= 32000 || remaining <= 0 {
		return nil
	}
	c.bytes += len(raw)
	if ctx == nil {
		ctx = context.Background()
	}
	ctx, cancel := context.WithTimeout(ctx, remaining)
	defer cancel()
	started := time.Now()
	result := syntax.Tokenize(ctx, key.path, raw)
	c.elapsed += time.Since(started)
	count := 0
	for _, spans := range result {
		count += len(spans)
	}
	if c.spans+count > 32000 {
		return nil
	}
	c.spans += count
	c.entries[key] = result
	return result
}
