package commits

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"github.com/pjbgf/sha1cd"
	"prui/internal/source"
	"regexp"
	"strings"
	"unicode/utf8"
)

const EmptyTreeSHA = "4b825dc642cb6eb9a060e54bf8d69288fbee4904"

var compositionOID = regexp.MustCompile(`^[0-9a-f]{40}$`)

func captureComposition(ctx context.Context, b *Bundle, o *captureObjects, l source.Limits) *Composition {
	fail := func() *Composition {
		return &Composition{Status: Unavailable, Reason: "Frozen composition source unavailable or capture budget exhausted."}
	}
	c := &Composition{Status: Captured, Trees: map[string][]TreeEntry{}, Blobs: map[string][]byte{}}
	paths, trees := map[string]bool{}, map[string]bool{}
	for _, e := range b.Entries {
		if e.Status != Captured || e.Diff == nil || !e.Diff.Complete {
			return fail()
		}
		trees[e.SHA] = true
		parent := EmptyTreeSHA
		if len(e.Parents) > 0 {
			parent = e.Parents[0]
		}
		trees[parent] = true
		for _, f := range e.Diff.Files {
			if len(f.OldPath) > 0 {
				paths[string(f.OldPath)] = true
			}
			if len(f.NewPath) > 0 {
				paths[string(f.NewPath)] = true
			}
		}
	}
	if len(paths) > min(l.Entries, 10000) {
		return fail()
	}
	for tree := range trees {
		c.Trees[tree] = []TreeEntry{}
		if tree == EmptyTreeSHA {
			continue
		}
		raw, err := o.Git(ctx, min(o.remaining, 50<<20), "ls-tree", "-r", "-z", "--full-tree", tree, "--")
		if err != nil {
			return fail()
		}
		for _, record := range bytes.Split(raw, []byte{0}) {
			if len(record) == 0 {
				continue
			}
			parts := bytes.SplitN(record, []byte{'\t'}, 2)
			if len(parts) != 2 {
				return fail()
			}
			if !paths[string(parts[1])] {
				continue
			}
			fields := strings.Fields(string(parts[0]))
			if len(fields) != 3 {
				return fail()
			}
			e := TreeEntry{Path: bytes.Clone(parts[1]), Mode: fields[0], OID: fields[2]}
			c.Trees[tree] = append(c.Trees[tree], e)
			if e.Mode == "160000" {
				continue
			}
			if _, ok := c.Blobs[e.OID]; !ok {
				data, err := o.Blob(ctx, e.OID, min(l.BlobBytes, 1<<20))
				if err != nil {
					return fail()
				}
				c.Blobs[e.OID] = data
			}
		}
	}

	encoded, err := json.Marshal(c)
	if err != nil || len(encoded) > o.remaining {
		return fail()
	}
	// Raw reads are already charged; encoded structural overhead also consumes
	// the shared budget. Conservatively charging encoding avoids oversize saves.
	o.remaining -= len(encoded)
	return c
}

// ValidateComposition checks all frozen references and Git blob identities without
// consulting the repository. Legacy bundles deliberately carry no extension.
func ValidateComposition(b *Bundle) bool {
	if b == nil || b.Composition == nil {
		return true
	}
	c := b.Composition
	if c.Status == Unavailable {
		return c.Reason != "" && len(c.Reason) <= 4096 && utf8.ValidString(c.Reason) && len(c.Trees) == 0 && len(c.Blobs) == 0
	}
	if c.Status != Captured || c.Reason != "" || b.Status != Captured || len(c.Trees) > 200 || len(c.Blobs) > 20000 {
		return false
	}
	paths, needed, refs := map[string]bool{}, map[string]bool{}, map[string]bool{}
	for _, e := range b.Entries {
		if e.Status != Captured || e.Diff == nil || !e.Diff.Complete {
			return false
		}
		needed[e.SHA] = true
		parent := EmptyTreeSHA
		if len(e.Parents) > 0 {
			parent = e.Parents[0]
		}
		needed[parent] = true
		for _, f := range e.Diff.Files {
			if len(f.OldPath) > 0 {
				paths[string(f.OldPath)] = true
			}
			if len(f.NewPath) > 0 {
				paths[string(f.NewPath)] = true
			}
		}
	}
	if len(paths) > 10000 || len(needed) != len(c.Trees) {
		return false
	}
	size := 0
	for tree, entries := range c.Trees {
		if !needed[tree] || !compositionOID.MatchString(tree) || len(entries) > 10000 {
			return false
		}
		seen := map[string]bool{}
		for _, e := range entries {
			p := string(e.Path)
			if !paths[p] || !safeCompositionPath(p) || seen[p] || !compositionOID.MatchString(e.OID) {
				return false
			}
			seen[p] = true

			switch e.Mode {
			case "100644", "100755", "120000":
				if _, ok := c.Blobs[e.OID]; !ok {
					return false
				}
				refs[e.OID] = true
			case "160000":
			default:
				return false
			}
			size += len(e.Path) + len(e.Mode) + len(e.OID)
		}
		for path := range seen {
			parts := strings.Split(path, "/")
			for i := 1; i < len(parts); i++ {
				if seen[strings.Join(parts[:i], "/")] {
					return false
				}
			}
		}
		if tree == EmptyTreeSHA && len(entries) > 0 {
			return false
		}
	}

	for _, entry := range b.Entries {
		parent := EmptyTreeSHA
		if len(entry.Parents) > 0 {
			parent = entry.Parents[0]
		}
		oldEntries, newEntries := map[string]TreeEntry{}, map[string]TreeEntry{}
		for _, e := range c.Trees[parent] {
			oldEntries[string(e.Path)] = e
		}
		for _, e := range c.Trees[entry.SHA] {
			newEntries[string(e.Path)] = e
		}
		expected := map[string]TreeEntry{}
		for path, e := range oldEntries {
			expected[path] = e
		}
		for _, f := range entry.Diff.Files {
			if len(f.OldPath) > 0 {
				delete(expected, string(f.OldPath))
			}
		}
		for _, f := range entry.Diff.Files {
			if len(f.NewPath) > 0 {
				if _, exists := expected[string(f.NewPath)]; exists {
					return false
				}
				expected[string(f.NewPath)] = TreeEntry{Path: f.NewPath, Mode: f.NewMode, OID: f.NewOID}
			}
		}
		if len(expected) != len(newEntries) {
			return false
		}
		for path, e := range expected {
			actual, ok := newEntries[path]
			if !ok || e.Mode != actual.Mode || e.OID != actual.OID {
				return false
			}
		}
		for _, f := range entry.Diff.Files {
			if len(f.OldPath) > 0 && (oldEntries[string(f.OldPath)].OID != f.OldOID || oldEntries[string(f.OldPath)].Mode != f.OldMode) {
				return false
			}
			if len(f.NewPath) > 0 && (newEntries[string(f.NewPath)].OID != f.NewOID || newEntries[string(f.NewPath)].Mode != f.NewMode) {
				return false
			}
		}
	}
	if len(refs) != len(c.Blobs) {
		return false
	}
	for oid, data := range c.Blobs {
		if len(data) > 1<<20 {
			return false
		}
		// Git blob identities use SHA-1; reject collision attacks explicitly.
		object := append([]byte(fmt.Sprintf("blob %d\x00", len(data))), data...)
		digest, collision := sha1cd.Sum(object)
		if collision || fmt.Sprintf("%x", digest) != oid {
			return false
		}
		size += len(oid) + len(data)
	}
	return size <= 50<<20
}
func safeCompositionPath(p string) bool {
	if p == "" || len(p) > 4096 || strings.Count(p, "/") >= 128 || strings.HasPrefix(p, "/") || strings.ContainsRune(p, 0) {
		return false
	}
	for _, part := range strings.Split(p, "/") {
		if part == "" || part == "." || part == ".." || strings.EqualFold(part, ".git") {
			return false
		}
	}
	return true
}
