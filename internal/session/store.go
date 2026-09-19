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
	"pr-review/internal/source"
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
	Checkout  []byte
	Inventory inventory.Inventory
	Slices    []Slice
	UnitFiles []int
	Context   reviewcontext.ContextBundle
	// Guides is a pointer so a session created before guide analysis existed
	// re-marshals to identical bytes and keeps its snapshot reference valid.
	Guides *guide.Bundle `json:"guides,omitempty"`
	// DerivedFrom links a guided copy to its immutable source session.
	DerivedFrom string `json:"derived_from,omitempty"`
}

type State struct {
	SchemaVersion     int            `json:"schema_version"`
	ID                string         `json:"session_id"`
	SnapshotReference string         `json:"snapshot_reference"`
	ReviewedSliceIDs  []string       `json:"reviewed_slice_ids"`
	RevisionStatus    RevisionStatus `json:"revision_status"`
	UpdatedAt         time.Time      `json:"updated_at"`
	Generation        uint64         `json:"generation"`
}

// legacyPlanState is read only to discard completion markers for the retired
// editable-plan feature when opening an older session. File-slice markers are
// retained; plan-only markers have no equivalent in the streamlined review.
type legacyPlanState struct {
	AcceptedPlan *struct {
		Slices []struct {
			SliceID string `json:"slice_id"`
		} `json:"slices"`
	} `json:"accepted_plan"`
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
var shaPattern = regexp.MustCompile(`^[0-9a-f]{40}$`)

var ErrRepositoryNotFound = errors.New("remembered repository not found")

type repositoryRecord struct {
	Repositories []Repository `json:"repositories"`
}

type Repository struct {
	Repository string `json:"repository"`
	Checkout   string `json:"checkout"`
}

// GuideCacheKey identifies the immutable comparison a generated guide
// describes. Both base and head revisions matter because either can change the
// review units for the same pull request.
type GuideCacheKey struct {
	Repository string
	Number     int
	BaseSHA    string
	HeadSHA    string
}

type guideCacheRecord struct {
	Repository string       `json:"repository"`
	Number     int          `json:"number"`
	BaseSHA    string       `json:"base_sha"`
	HeadSHA    string       `json:"head_sha"`
	Bundle     guide.Bundle `json:"bundle"`
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
		_ = lock.Close()
		return nil, errors.New("session storage busy: another pr-review process holds the writer lock")
	}
	if err := storageFormat(path); err != nil {
		_ = lock.Close()
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

func normalizeGuideCacheKey(key GuideCacheKey) (GuideCacheKey, error) {
	repository, err := normalizeRepository(key.Repository)
	if err != nil || key.Number <= 0 || !shaPattern.MatchString(key.BaseSHA) || !shaPattern.MatchString(key.HeadSHA) {
		return GuideCacheKey{}, errors.New("invalid guide cache key")
	}
	key.Repository = repository
	return key, nil
}

func guideCacheMatchesInventory(key GuideCacheKey, inv inventory.Inventory) bool {
	comparison := inv.Comparison.Metadata
	repository, err := normalizeRepository(comparison.Identity.Repository)
	return err == nil && repository == key.Repository && comparison.Identity.Number == key.Number && comparison.BaseSHA == key.BaseSHA && comparison.HeadSHA == key.HeadSHA
}

func (s *Store) guideCacheDirectory() string { return filepath.Join(s.path, "guides") }

func (s *Store) guideCachePath(key GuideCacheKey) string {
	// A hash makes the on-disk filename fixed-width and prevents untrusted
	// repository metadata from becoming a path component. The record repeats
	// the canonical identity so reads can detect a misplaced artifact.
	b, _ := json.Marshal(key)
	return filepath.Join(s.guideCacheDirectory(), digest(b)+".json")
}

// SaveGeneratedGuide atomically persists a generated guide for an immutable
// comparison. Unavailable or structurally invalid bundles are deliberately not
// cached so a later PR-list open can retry analysis.
func (s *Store) SaveGeneratedGuide(key GuideCacheKey, bundle guide.Bundle, inv inventory.Inventory) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if err := s.writable(); err != nil {
		return err
	}
	key, err := normalizeGuideCacheKey(key)
	if err != nil {
		return err
	}
	if !guideCacheMatchesInventory(key, inv) {
		return errors.New("guide cache key does not match inventory comparison")
	}
	if bundle.Status != guide.Generated || guide.Validate(bundle, inv) != nil {
		return errors.New("only valid generated guides may be cached")
	}
	dir := s.guideCacheDirectory()
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err := privatePath(dir, true); err != nil {
		return err
	}
	b, err := json.Marshal(guideCacheRecord{Repository: key.Repository, Number: key.Number, BaseSHA: key.BaseSHA, HeadSHA: key.HeadSHA, Bundle: bundle})
	if err != nil {
		return err
	}
	return atomicWrite(dir, filepath.Base(s.guideCachePath(key)), b)
}

// LoadGeneratedGuide returns a valid generated guide for key, or nil on a
// cache miss. Corrupt, unsafe, mismatched, and inventory-invalid artifacts are
// treated as misses and are left in place for inspection rather than replaced.
func (s *Store) LoadGeneratedGuide(key GuideCacheKey, inv inventory.Inventory) (*guide.Bundle, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || (!s.readOnly && s.lock == nil) {
		return nil, errors.New("session store closed")
	}
	key, err := normalizeGuideCacheKey(key)
	if err != nil {
		return nil, err
	}
	if !guideCacheMatchesInventory(key, inv) {
		return nil, nil
	}
	dir := s.guideCacheDirectory()
	if err := privatePath(dir, true); err != nil {
		return nil, nil
	}
	var cached guideCacheRecord
	if _, err := readJSON(s.guideCachePath(key), &cached); err != nil {
		return nil, nil
	}
	cachedKey, err := normalizeGuideCacheKey(GuideCacheKey{Repository: cached.Repository, Number: cached.Number, BaseSHA: cached.BaseSHA, HeadSHA: cached.HeadSHA})
	if err != nil || cachedKey != key || cached.Bundle.Status != guide.Generated || guide.Validate(cached.Bundle, inv) != nil {
		return nil, nil
	}
	bundle := cached.Bundle
	return &bundle, nil
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
	defer func() { _ = f.Close() }()
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
	defer func() { _ = os.Remove(f.Name()) }()
	defer func() { _ = f.Close() }()
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
	defer func() { _ = f.Close() }()
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

// HasComparisonSnapshot reports whether any valid local frozen snapshot exists
// for a pull request. It lets callers avoid a metadata request for PRs that
// have never been opened locally.
func (s *Store) HasComparisonSnapshot(id source.Identity) (bool, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || (!s.readOnly && s.lock == nil) {
		return false, errors.New("session store closed")
	}
	repository, err := normalizeRepository(id.Repository)
	if err != nil || id.Number <= 0 {
		return false, errors.New("invalid pull request identity")
	}
	entries, err := os.ReadDir(s.path)
	if err != nil {
		return false, err
	}
	for _, entry := range entries {
		if !idPattern.MatchString(entry.Name()) {
			continue
		}
		r, err := s.load(entry.Name())
		if err != nil {
			continue
		}
		comparison := r.Inventory.Comparison.Metadata.Identity
		cachedRepository, err := normalizeRepository(comparison.Repository)
		if err == nil && cachedRepository == repository && comparison.Number == id.Number {
			return true, nil
		}
	}
	return false, nil
}

// LoadComparisonSnapshot returns a previously validated frozen snapshot for
// the exact immutable comparison metadata, or nil when none is available. It is a
// local source cache: callers must first obtain fresh PR metadata and match
// both base and head revisions before using it.
//
// Unreadable sessions are treated as cache misses and deliberately left in
// place, matching guide-cache behavior. The returned snapshot is a value, so
// callers can safely update invocation-specific hints such as Checkout before
// creating a new session.
func (s *Store) LoadComparisonSnapshot(metadata source.Metadata) (*Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.closed || (!s.readOnly && s.lock == nil) {
		return nil, errors.New("session store closed")
	}
	key := GuideCacheKey{Repository: metadata.Identity.Repository, Number: metadata.Identity.Number, BaseSHA: metadata.BaseSHA, HeadSHA: metadata.HeadSHA}
	if _, err := normalizeGuideCacheKey(key); err != nil {
		return nil, err
	}
	entries, err := os.ReadDir(s.path)
	if err != nil {
		return nil, err
	}
	for _, entry := range entries {
		if !idPattern.MatchString(entry.Name()) {
			continue
		}
		r, err := s.load(entry.Name())
		if err != nil || !guideCacheMatchesInventory(key, r.Inventory) {
			continue
		}
		cached := r.Inventory.Comparison.Metadata
		if !strings.EqualFold(cached.BaseRepository, metadata.BaseRepository) || !strings.EqualFold(cached.HeadRepository, metadata.HeadRepository) {
			continue
		}
		snapshot := r.Snapshot
		// Guides and their parent links are derived interpretation, not source
		// material. The caller resolves the guide cache separately.
		snapshot.Guides = nil
		snapshot.DerivedFrom = ""
		return &snapshot, nil
	}
	return nil, nil
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
	state, err := readJSON(filepath.Join(dir, "state.json"), &r.State)
	if err != nil {
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
	var legacy legacyPlanState
	if json.Unmarshal(state, &legacy) == nil && legacy.AcceptedPlan != nil {
		files := make(map[string]bool, len(r.Slices))
		for _, slice := range r.Slices {
			files[slice.FileID] = true
		}
		kept := r.ReviewedSliceIDs[:0]
		for _, id := range r.ReviewedSliceIDs {
			if files[id] {
				kept = append(kept, id)
			}
		}
		r.ReviewedSliceIDs = kept
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
	if r.SchemaVersion != SchemaVersion || r.Inventory.Comparison.InventoryID == "" || r.Generation == 0 {
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
	seen := map[string]bool{}
	for _, id := range r.ReviewedSliceIDs {
		if !files[id] || seen[id] {
			return bad
		}
		seen[id] = true
	}
	if r.Guides != nil && guide.Validate(*r.Guides, r.Inventory) != nil {
		return bad
	}
	return nil
}
