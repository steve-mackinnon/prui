package guide

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	reviewcontext "pr-review/internal/context"
	"pr-review/internal/inventory"
	"pr-review/internal/privacy"
)

const SearchVersion = "local-search-v1"

// Preparation is local, attempt-owned consent state. It is never persisted.
// Only Preview's initial input and later bounded tool results may be uploaded.
type Preparation struct {
	Input             Input
	Corpus            *reviewcontext.SearchCorpus
	UnavailableReason string
	Recipient, Model  string
	inventory         inventory.Inventory
	saved             reviewcontext.ContextBundle
	policy            privacy.Policy
	excluded          map[string]bool
	approved          string
}

func NewPreparation(inv inventory.Inventory, saved reviewcontext.ContextBundle, corpus *reviewcontext.SearchCorpus, policy privacy.Policy) *Preparation {
	p := &Preparation{inventory: inv, saved: saved, policy: policy, excluded: map[string]bool{}}
	if corpus != nil {
		p.Corpus = &reviewcontext.SearchCorpus{Files: append([]reviewcontext.SearchFile(nil), corpus.Files...), Omissions: append([]reviewcontext.Omitted(nil), corpus.Omissions...), Incomplete: corpus.Incomplete}
	}
	p.rebuild()
	return p
}

// Preview returns the input rebuilt when the approved scope last changed.
func (p *Preparation) Preview() Input { return p.Input }

func (p *Preparation) rebuild() Input {
	denied := map[string]string{}
	deny := func(path []byte, reason string) {
		if len(path) > 0 {
			denied[string(path)] = reason
		}
	}
	for path := range p.excluded {
		denied[path] = "user exclusion: selected file"
	}
	c := p.saved
	c.OmittedPaths = append([]reviewcontext.Omitted(nil), c.OmittedPaths...)
	if p.Corpus != nil {
		c = reviewcontext.ContextBundle{OmittedPaths: append([]reviewcontext.Omitted(nil), p.Corpus.Omissions...)}
		eligible := map[string]bool{}
		for _, f := range p.Corpus.Files {
			if ok, reason := p.policy.Allows(f.Evidence.Path, f.Evidence.Excerpt); !ok {
				denied[string(f.Evidence.Path)] = reason
				continue
			}
			eligible[f.Revision+"\x00"+string(f.Evidence.Path)+"\x00"+f.Evidence.BlobID] = true
		}
		for _, f := range p.inventory.Files {
			if len(f.OldPath) > 0 && !eligible["merge_base\x00"+string(f.OldPath)+"\x00"+f.OldOID] {
				denied[string(f.OldPath)] = "source unavailable for search"
			}
			if len(f.NewPath) > 0 && !eligible["head\x00"+string(f.NewPath)+"\x00"+f.NewOID] {
				denied[string(f.NewPath)] = "source unavailable for search"
			}
		}
	}
	patchDenials := map[string]string{}
	for _, u := range p.inventory.Units {
		if yes, reason := p.policy.ExcludedContent(p.inventory.Patches[u.PatchReference]); yes {
			patchDenials[u.FileChangeID] = reason
		}
	}
	for _, f := range p.inventory.Files {
		if yes, reason := excludedPath(p.policy, f); yes {
			deny(f.OldPath, reason)
			deny(f.NewPath, reason)
		}
		if reason := patchDenials[f.ID]; reason != "" {
			deny(f.OldPath, reason)
			deny(f.NewPath, reason)
		}
	}
	for _, o := range c.OmittedPaths {
		if o.Reason == "credential-like content" || o.Reason == "credential-like filename" || o.Reason == "binary content" {
			deny(o.Path, o.Reason)
		}
	}

	// Closing aliases before selecting tools prevents a patch exclusion from
	// being bypassed by reading the same file through its old/new name.
	byPath := map[string][]inventory.FileChange{}
	var pending []string
	queued := map[string]bool{}
	for _, f := range p.inventory.Files {
		for _, path := range [][]byte{f.OldPath, f.NewPath} {
			if len(path) > 0 {
				key := string(path)
				byPath[key] = append(byPath[key], f)
				if denied[key] != "" && !queued[key] {
					pending = append(pending, key)
					queued[key] = true
				}
			}
		}
	}
	for i := 0; i < len(pending); i++ {
		path := pending[i]
		for _, f := range byPath[path] {
			for _, alias := range [][]byte{f.OldPath, f.NewPath} {
				if len(alias) > 0 && denied[string(alias)] == "" {
					denied[string(alias)] = denied[path]
					pending = append(pending, string(alias))
				}
			}
		}
	}

	// Inventory order and corpus order keep omission/digest output deterministic.
	added := map[string]bool{}
	add := func(path []byte) {
		if reason := denied[string(path)]; len(path) > 0 && reason != "" && !added[string(path)] {
			c.OmittedPaths = append(c.OmittedPaths, reviewcontext.Omitted{Path: bytes.Clone(path), Reason: reason})
			added[string(path)] = true
		}
	}
	for _, f := range p.inventory.Files {
		add(f.OldPath)
		add(f.NewPath)
	}
	if p.Corpus != nil {
		for _, f := range p.Corpus.Files {
			add(f.Evidence.Path)
		}
	} else {
		for _, e := range c.Evidence {
			add(e.Path)
		}
	}
	in := InputFrom(p.inventory, c, p.policy, Defaults)
	if p.Corpus != nil {
		filtered := &reviewcontext.SearchCorpus{Incomplete: p.Corpus.Incomplete, Omissions: c.OmittedPaths}
		for _, f := range p.Corpus.Files {
			if denied[string(f.Evidence.Path)] == "" {
				filtered.Files = append(filtered.Files, f)
			}
		}
		in.Search = filtered
	}
	p.Input = in
	return in
}

func (p *Preparation) Files() []reviewcontext.SearchFile {
	in := p.Preview()
	if in.Search != nil {
		return append([]reviewcontext.SearchFile(nil), in.Search.Files...)
	}
	var files []reviewcontext.SearchFile
	listed := map[string]bool{}
	for _, e := range in.Evidence {
		listed[string(e.Path)] = true
		files = append(files, reviewcontext.SearchFile{Revision: "saved", Evidence: e})
	}
	for _, u := range in.Units {
		if listed[string(u.Path)] {
			continue
		}
		listed[string(u.Path)] = true
		var patch []byte
		for _, other := range in.Units {
			if bytes.Equal(u.Path, other.Path) {
				patch = append(patch, other.Patch...)
			}
		}
		files = append(files, reviewcontext.SearchFile{Revision: "saved patches", Evidence: reviewcontext.Evidence{Path: bytes.Clone(u.Path), Excerpt: patch}})
	}
	return files
}
func (p *Preparation) Exclude(path string) error {
	if path == "" {
		return errors.New("select a source file to exclude")
	}
	p.excluded[path] = true
	p.approved = ""
	p.rebuild()
	return nil
}
func (p *Preparation) Digest() string {
	in := p.Preview()
	h := sha256.New()
	field(h, []byte(in.digest()), []byte(p.Recipient), []byte(p.Model), []byte(SearchVersion), []byte(p.UnavailableReason))
	if in.Search != nil {
		field(h, []byte(fmt.Sprint(in.Search.Incomplete)))
		for _, f := range in.Search.Files {
			e := f.Evidence
			field(h, []byte(f.Revision), []byte(e.Repository), []byte(e.CommitSHA), []byte(e.BlobID), e.Path, e.Excerpt)
		}
	}
	return hex.EncodeToString(h.Sum(nil))
}
func (p *Preparation) Confirm() { p.approved = p.Digest() }
func (p *Preparation) ValidateApproval() error {
	if p == nil || p.approved == "" || p.approved != p.Digest() {
		return errors.New("guide source changed; inspect and confirm again")
	}
	return nil
}
