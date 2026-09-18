package session

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strings"
	"sync"
	"syscall"
	"time"

	reviewcontext "pr-review/internal/context"
	"pr-review/internal/guide"
	"pr-review/internal/inventory"
	"pr-review/internal/plan"
)

const SchemaVersion = 1
const maxJSONBytes = 128 << 20

type RevisionStatus string

const (
	Unchecked   RevisionStatus = "unchecked"
	Current     RevisionStatus = "current"
	Stale       RevisionStatus = "stale"
	CheckFailed RevisionStatus = "check_failed"
)

type Slice struct {
	FileID string
	Units  []int
}

type Snapshot struct {
	Checkout     []byte
	Inventory    inventory.Inventory
	PlanVersion  string
	Slices       []Slice
	UnitFiles    []int
	Context      reviewcontext.ContextBundle
	AnalysisPlan *plan.ValidatedPlan
	// Guides is a pointer so a session created before guide analysis existed
	// re-marshals to identical bytes and keeps its snapshot reference valid.
	Guides *guide.Bundle `json:"guides,omitempty"`
	// DerivedFrom links a guided copy to its immutable source session.
	DerivedFrom string `json:"derived_from,omitempty"`
}

type State struct {
	SchemaVersion     int                 `json:"schema_version"`
	ID                string              `json:"session_id"`
	SnapshotReference string              `json:"snapshot_reference"`
	ReviewedSliceIDs  []string            `json:"reviewed_slice_ids"`
	RevisionStatus    RevisionStatus      `json:"revision_status"`
	UpdatedAt         time.Time           `json:"updated_at"`
	Generation        uint64              `json:"generation"`
	AcceptedPlan      *plan.ValidatedPlan `json:"accepted_plan,omitempty"`
	PreviousPlan      *plan.ValidatedPlan `json:"previous_plan,omitempty"`
}

// CurrentPlan returns the most recently accepted editable plan.
func (r *Record) CurrentPlan() *plan.ValidatedPlan {
	if r.AcceptedPlan != nil {
		return r.AcceptedPlan
	}
	return r.AnalysisPlan
}

// ApplyPlan persists a validated replacement and conservatively clears progress.
// The immutable inventory and original snapshot remain untouched.
func (s *Store) ApplyPlan(r *Record, replacement plan.ValidatedPlan) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return err
	}
	unitIDs, evidenceIDs := map[string]bool{}, map[string]bool{}
	for _, u := range r.Inventory.Units {
		unitIDs[u.ID] = true
	}
	for _, e := range r.Context.Evidence {
		evidenceIDs[e.EvidenceID] = true
	}
	proposal := plan.Proposal{InventoryID: replacement.InventoryID, Slices: replacement.Slices}
	validated, err := plan.Validate(proposal, r.Inventory.Comparison.InventoryID, unitIDs, evidenceIDs)
	if err != nil {
		return err
	}
	validated.Version = replacement.Version
	validated.AnalysisStatus = replacement.AnalysisStatus
	validated.Warnings = append([]string(nil), replacement.Warnings...)
	validated.Provenance = replacement.Provenance
	old, err := s.load(r.ID)
	if err != nil {
		return err
	}
	if r.Generation != old.Generation {
		return errors.New("outdated writer; reload or create a new session")
	}
	next := r.State
	previous := old.CurrentPlan()
	if previous == nil {
		fallback := plan.FileFallback(old.Inventory)
		previous = &fallback
	}
	previousCopy := *previous
	next.PreviousPlan = &previousCopy
	next.AcceptedPlan = &validated
	next.ReviewedSliceIDs = []string{}
	next.Generation++
	next.UpdatedAt = time.Now().UTC()
	if err := validate(&Record{Snapshot: old.Snapshot, State: next}); err != nil {
		return err
	}
	b, _ := json.Marshal(next)
	if err := atomicWrite(filepath.Join(s.path, r.ID), "state.json", b); err != nil {
		return err
	}
	r.State = next
	return nil
}

type Record struct {
	Snapshot
	State
}

type Entry struct {
	ID     string
	Record *Record
	Err    error
}

type Store struct {
	mu       sync.Mutex
	path     string
	lock     *os.File
	readOnly bool
	closed   bool
}

var idPattern = regexp.MustCompile(`^[0-9a-f]{32}$`)
var repositoryPattern = regexp.MustCompile(`^[A-Za-z0-9][A-Za-z0-9_-]*/[A-Za-z0-9][A-Za-z0-9_.-]*$`)

var ErrRepositoryNotFound = errors.New("remembered repository not found")

type repositoryRecord struct {
	Repositories []Repository `json:"repositories"`
}

type Repository struct {
	Repository string `json:"repository"`
	Checkout   string `json:"checkout"`
}

func DefaultPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	if runtime.GOOS == "darwin" {
		return filepath.Join(home, "Library", "Application Support", "pr-review", "sessions"), nil
	}
	base := os.Getenv("XDG_DATA_HOME")
	if !filepath.IsAbs(base) {
		base = filepath.Join(home, ".local", "share")
	}
	return filepath.Join(base, "pr-review", "sessions"), nil
}

func Open(path string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err = os.MkdirAll(path, 0700); err != nil {
		return nil, err
	}
	if err = privatePath(path, true); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return nil, err
	}
	if _, err := os.Lstat(filepath.Join(path, ".format")); os.IsNotExist(err) {
		for _, entry := range entries {
			if entry.Name() != ".lock" {
				return nil, errors.New("refusing nonempty directory without pr-review storage marker")
			}
		}
	}
	lockPath := filepath.Join(path, ".lock")
	if err := privatePath(lockPath, false); err != nil && !os.IsNotExist(err) {
		return nil, err
	}
	lock, err := os.OpenFile(filepath.Join(path, ".lock"), os.O_CREATE|os.O_RDWR|syscall.O_NOFOLLOW, 0600)
	if err != nil {
		return nil, err
	}
	if err = syscall.Flock(int(lock.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		lock.Close()
		return nil, errors.New("session storage busy: another pr-review process holds the writer lock")
	}
	if err := storageFormat(path); err != nil {
		lock.Close()
		return nil, err
	}
	return &Store{path: path, lock: lock}, nil
}

// OpenReadOnly opens an existing, app-owned store without creating files,
// directories, or acquiring the writer lock. Mutating Store methods reject the
// returned handle.
func OpenReadOnly(path string) (*Store, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	if err := privatePath(path, true); err != nil {
		return nil, err
	}
	if err := readStorageFormat(path); err != nil {
		return nil, err
	}
	return &Store{path: path, readOnly: true}, nil
}

func storageFormat(path string) error {
	type format struct {
		Application string
		Version     int
	}
	expected := format{"pr-review", SchemaVersion}
	marker := filepath.Join(path, ".format")
	if _, err := os.Lstat(marker); os.IsNotExist(err) {
		b, _ := json.Marshal(expected)
		return atomicWrite(path, ".format", b)
	}
	return readStorageFormat(path)
}

func readStorageFormat(path string) error {
	type format struct {
		Application string
		Version     int
	}
	expected := format{"pr-review", SchemaVersion}
	var got format
	if _, err := readJSON(filepath.Join(path, ".format"), &got); err != nil {
		return err
	}
	if got != expected {
		return errors.New("unsupported storage format; original retained")
	}
	return nil
}

func (s *Store) Path() string { return s.path }

func (s *Store) writable() error {
	if s.closed {
		return errors.New("session store closed")
	}
	if s.readOnly {
		return errors.New("session store is read-only")
	}
	return nil
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed {
		return nil
	}
	s.closed = true
	if s.lock == nil {
		return nil
	}
	err := s.lock.Close()
	s.lock = nil
	return err
}

// LookupRepository returns the canonical checkout remembered for a repository.
func (s *Store) LookupRepository(repository string) (string, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return "", errors.New("session store closed")
	}
	repository, err := normalizeRepository(repository)
	if err != nil {
		return "", err
	}
	r, err := s.repositories()
	if err != nil {
		return "", err
	}
	for _, entry := range r.Repositories {
		if entry.Repository == repository {
			return entry.Checkout, nil
		}
	}
	return "", ErrRepositoryNotFound
}

// ListRepositories returns remembered repositories in their durable order.
func (s *Store) ListRepositories() ([]Repository, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, errors.New("session store closed")
	}
	r, err := s.repositories()
	if err != nil {
		return nil, err
	}
	return append([]Repository(nil), r.Repositories...), nil
}

// RememberRepository replaces the one canonical checkout hint for a repository.
func (s *Store) RememberRepository(repository, checkout string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return err
	}
	if s.lock == nil {
		return errors.New("session store closed")
	}
	repository, err := normalizeRepository(repository)
	if err != nil {
		return err
	}
	if !filepath.IsAbs(checkout) || filepath.Clean(checkout) != checkout {
		return errors.New("repository checkout must be a canonical absolute path")
	}
	r, err := s.repositories()
	if err != nil {
		return err
	}
	updated := false
	for i := range r.Repositories {
		if r.Repositories[i].Repository == repository {
			r.Repositories[i].Checkout = checkout
			updated = true
		}
	}
	if !updated {
		r.Repositories = append(r.Repositories, Repository{Repository: repository, Checkout: checkout})
	}
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	return atomicWrite(s.path, "repositories.json", b)
}

func (s *Store) repositories() (repositoryRecord, error) {
	path := filepath.Join(s.path, "repositories.json")
	if _, err := os.Lstat(path); os.IsNotExist(err) {
		return repositoryRecord{}, nil
	}
	var r repositoryRecord
	if _, err := readJSON(path, &r); err != nil {
		return repositoryRecord{}, err
	}
	seen := map[string]bool{}
	for _, entry := range r.Repositories {
		if entry.Repository != strings.ToLower(entry.Repository) || !repositoryPattern.MatchString(entry.Repository) || seen[entry.Repository] || !filepath.IsAbs(entry.Checkout) || filepath.Clean(entry.Checkout) != entry.Checkout {
			return repositoryRecord{}, errors.New("invalid remembered repository record; original retained")
		}
		seen[entry.Repository] = true
	}
	return r, nil
}

func normalizeRepository(repository string) (string, error) {
	if !repositoryPattern.MatchString(repository) {
		return "", errors.New("invalid GitHub repository")
	}
	return strings.ToLower(repository), nil
}

func privatePath(path string, directory bool) error {
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if info.Mode()&os.ModeSymlink != 0 || info.IsDir() != directory || (!directory && !info.Mode().IsRegular()) || info.Mode().Perm()&0077 != 0 {
		return errors.New("session storage must contain private regular files and directories, not symlinks")
	}
	return nil
}

func (s *Store) directory(id string) (string, error) {
	if s.closed || (!s.readOnly && s.lock == nil) {
		return "", errors.New("session store closed")
	}
	if !idPattern.MatchString(id) {
		return "", errors.New("invalid session ID")
	}
	return filepath.Join(s.path, id), nil
}

func digest(b []byte) string { return fmt.Sprintf("%x", sha256.Sum256(b)) }

func readJSON(path string, value any) ([]byte, error) {
	if err := privatePath(path, false); err != nil {
		return nil, err
	}
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	b, err := io.ReadAll(io.LimitReader(f, maxJSONBytes+1))
	if err != nil {
		return nil, err
	}
	if len(b) > maxJSONBytes {
		return nil, errors.New("session JSON exceeds storage limit")
	}
	d := json.NewDecoder(bytes.NewReader(b))
	d.DisallowUnknownFields()
	if err := d.Decode(value); err != nil {
		return nil, err
	}
	if err := d.Decode(new(any)); err != io.EOF {
		return nil, errors.New("trailing session JSON")
	}
	return b, nil
}

func atomicWrite(dir, name string, b []byte) error {
	if len(b) > maxJSONBytes {
		return errors.New("session JSON exceeds storage limit")
	}
	f, err := os.CreateTemp(dir, ".write-")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if _, err = f.Write(b); err != nil {
		return err
	}
	if err = f.Sync(); err != nil {
		return err
	}
	if err = f.Close(); err != nil {
		return err
	}
	if err = os.Rename(f.Name(), filepath.Join(dir, name)); err != nil {
		return err
	}
	return syncDir(dir)
}

func syncDir(dir string) error {
	f, err := os.Open(dir)
	if err != nil {
		return err
	}
	defer f.Close()
	return f.Sync()
}

func (s *Store) Create(snapshot Snapshot) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return nil, err
	}
	if err := s.validateDerived(snapshot); err != nil {
		return nil, err
	}
	b, err := json.Marshal(snapshot)
	if err != nil {
		return nil, err
	}
	r := &Record{Snapshot: snapshot, State: State{SchemaVersion: SchemaVersion, ID: fmt.Sprintf("%x", randBytes()), SnapshotReference: digest(b), ReviewedSliceIDs: []string{}, RevisionStatus: Unchecked, UpdatedAt: time.Now().UTC(), Generation: 1}}
	if err := validate(r); err != nil {
		return nil, err
	}
	dir, err := s.directory(r.ID)
	if err != nil {
		return nil, err
	}
	if err = os.Mkdir(dir, 0700); err != nil {
		return nil, err
	}
	if err = atomicWrite(dir, "snapshot.json", b); err != nil {
		return nil, err
	}
	state, _ := json.Marshal(r.State)
	if err = atomicWrite(dir, "state.json", state); err != nil {
		return nil, err
	}
	if err = syncDir(s.path); err != nil {
		return nil, err
	}
	return s.load(r.ID)
}

func (s *Store) validateDerived(snapshot Snapshot) error {
	if snapshot.DerivedFrom == "" {
		return nil
	}
	if !idPattern.MatchString(snapshot.DerivedFrom) {
		return errors.New("invalid derived session source")
	}
	parent, err := s.load(snapshot.DerivedFrom)
	if err != nil {
		return fmt.Errorf("derived session source unavailable: %w", err)
	}
	derivedRaw, parentRaw := snapshot, parent.Snapshot
	derivedRaw.Guides, derivedRaw.DerivedFrom = nil, ""
	parentRaw.Guides, parentRaw.DerivedFrom = nil, ""
	derivedBytes, err := json.Marshal(derivedRaw)
	if err != nil {
		return err
	}
	parentBytes, err := json.Marshal(parentRaw)
	if err != nil {
		return err
	}
	if !bytes.Equal(derivedBytes, parentBytes) {
		return errors.New("derived session source evidence changed")
	}
	return nil
}

func randBytes() []byte { b := make([]byte, 16); _, _ = rand.Read(b); return b }

func (s *Store) Load(id string) (*Record, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.load(id)
}

func (s *Store) load(id string) (*Record, error) {
	dir, err := s.directory(id)
	if err != nil {
		return nil, err
	}
	if err = privatePath(dir, true); err != nil {
		return nil, err
	}
	r := &Record{}
	if _, err = readJSON(filepath.Join(dir, "state.json"), &r.State); err != nil {
		return nil, err
	}
	if r.SchemaVersion != SchemaVersion {
		return nil, errors.New("unsupported session schema; original retained")
	}
	if r.ID != id {
		return nil, errors.New("session ID mismatch")
	}
	b, err := readJSON(filepath.Join(dir, "snapshot.json"), &r.Snapshot)
	if err != nil {
		return nil, err
	}
	if digest(b) != r.SnapshotReference {
		return nil, errors.New("snapshot checksum mismatch; original retained")
	}
	if err := validate(r); err != nil {
		return nil, err
	}
	return r, nil
}

func (s *Store) Save(r *Record) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return err
	}
	old, err := s.load(r.ID)
	if err != nil {
		return err
	}
	b, err := json.Marshal(r.Snapshot)
	if err != nil {
		return err
	}
	if digest(b) != old.SnapshotReference || r.SnapshotReference != old.SnapshotReference || r.Generation != old.Generation {
		return errors.New("immutable snapshot changed or outdated writer; reload or create a new session")
	}
	if err := validate(r); err != nil {
		return err
	}
	next := r.State
	next.Generation++
	next.UpdatedAt = time.Now().UTC()
	b, _ = json.Marshal(next)
	if err = atomicWrite(filepath.Join(s.path, r.ID), "state.json", b); err != nil {
		return err
	}
	r.State = next
	return nil
}

func (s *Store) List() ([]Entry, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.lock == nil {
		return nil, errors.New("session store closed")
	}
	entries, err := os.ReadDir(s.path)
	if err != nil {
		return nil, err
	}
	result := []Entry{}
	for _, entry := range entries {
		if !idPattern.MatchString(entry.Name()) {
			continue
		}
		r, err := s.load(entry.Name())
		result = append(result, Entry{ID: entry.Name(), Record: r, Err: err})
	}
	return result, nil
}

func (s *Store) Delete(id string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return err
	}
	dir, err := s.directory(id)
	if err != nil {
		return err
	}
	if err = privatePath(dir, true); err != nil {
		return err
	}
	if err = os.RemoveAll(dir); err != nil {
		return err
	}
	return syncDir(s.path)
}

func validate(r *Record) error {
	bad := errors.New("invalid session references or progress; original retained")
	if r.SchemaVersion != SchemaVersion || r.PlanVersion == "" || r.Inventory.Comparison.InventoryID == "" || r.Generation == 0 {
		return bad
	}
	switch r.RevisionStatus {
	case Unchecked, Current, Stale, CheckFailed:
	default:
		return bad
	}
	if len(r.UnitFiles) != len(r.Inventory.Units) || len(r.Slices) != len(r.Inventory.Files) {
		return bad
	}
	files, units, owned := map[string]bool{}, map[string]bool{}, map[int]bool{}
	for i, slice := range r.Slices {
		if slice.FileID == "" || files[slice.FileID] || slice.FileID != r.Inventory.Files[i].ID || len(slice.Units) == 0 {
			return bad
		}
		files[slice.FileID] = true
		for _, u := range slice.Units {
			if u < 0 || u >= len(r.Inventory.Units) || owned[u] || r.UnitFiles[u] != i || r.Inventory.Units[u].FileChangeID != slice.FileID {
				return bad
			}
			owned[u] = true
		}
	}
	if len(owned) != len(r.Inventory.Units) {
		return bad
	}
	for _, u := range r.Inventory.Units {
		if u.ID == "" || units[u.ID] || u.InventoryID != r.Inventory.Comparison.InventoryID {
			return bad
		}
		units[u.ID] = true
		switch u.Kind {
		case inventory.TextHunk:
			if u.PatchReference == "" {
				return bad
			}
		case inventory.FileMetadata, inventory.Binary, inventory.Gitlink, inventory.Unavailable:
		default:
			return bad
		}
		if u.PatchReference != "" {
			if b, ok := r.Inventory.Patches[u.PatchReference]; !ok || digest(b) != u.PatchReference {
				return bad
			}
		}
	}
	planSliceIDs := map[string]bool{}
	if current := r.CurrentPlan(); current != nil {
		for _, slice := range current.Slices {
			planSliceIDs[slice.SliceID] = true
		}
	}
	seen := map[string]bool{}
	for _, id := range r.ReviewedSliceIDs {
		if (!files[id] && !planSliceIDs[id]) || seen[id] {
			return bad
		}
		seen[id] = true
	}
	if current := r.CurrentPlan(); current != nil {
		unitIDs, evidenceIDs := map[string]bool{}, map[string]bool{}
		for _, u := range r.Inventory.Units {
			unitIDs[u.ID] = true
		}
		for _, e := range r.Context.Evidence {
			evidenceIDs[e.EvidenceID] = true
		}
		proposal := plan.Proposal{InventoryID: current.InventoryID, Slices: current.Slices}
		if _, err := plan.Validate(proposal, r.Inventory.Comparison.InventoryID, unitIDs, evidenceIDs); err != nil {
			return bad
		}
	}
	if r.Guides != nil && guide.Validate(*r.Guides, r.Inventory) != nil {
		return bad
	}
	return nil
}
