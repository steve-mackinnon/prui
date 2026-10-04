package session

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"unicode/utf8"

	"prui/internal/session/storage"
	"prui/internal/source"
)

// DraftKey includes both repository identities and pins. Drafts are independent
// mutable local work, never snapshot payloads or guide input.
type DraftKey struct {
	Repository                                       string
	Number                                           int
	BaseSHA, HeadSHA, BaseRepository, HeadRepository string
}

func DraftKeyFor(m source.Metadata) DraftKey {
	return DraftKey{m.Identity.Repository, m.Identity.Number, m.BaseSHA, m.HeadSHA, m.BaseRepository, m.HeadRepository}
}

type DraftEditor struct {
	Suggestion           bool
	Before               string
	RootAnchor           *source.ReviewCommentTarget
	CommitSHA            string
	Target               source.ReviewCommentTarget
	Body                 string
	PendingIndex         int
	CommentID, ReplyToID int64
}
type DraftAttempt struct {
	Application *source.SuggestionApplication
	Kind        string
	Comment     *source.ReviewComment
	Review      *source.PullRequestReview
	ParentID    int64
}

type Draft struct {
	SuggestionApply *source.SuggestionApplication
	Attempted       *DraftAttempt
	Version         int
	Generation      uint64
	Pending         []source.ReviewComment
	Summary         string
	Event           int
	Composer, Reply *DraftEditor
	// Attempt is saved before dispatch. A crash or any failed response requires
	// remote reconciliation before another explicit submission can proceed.
	Attempt string
}

const draftSchema = `CREATE TABLE IF NOT EXISTS review_drafts (
 repository TEXT NOT NULL, pr_number INTEGER NOT NULL,
 base_sha TEXT NOT NULL, head_sha TEXT NOT NULL,
 base_repository TEXT NOT NULL, head_repository TEXT NOT NULL,
 generation INTEGER NOT NULL CHECK(generation>0),
 payload BLOB NOT NULL CHECK(length(payload) BETWEEN 1 AND 1048576),
 PRIMARY KEY(repository,pr_number,base_sha,head_sha,base_repository,head_repository)
) STRICT;`

func normalizeDraftKey(k DraftKey) (DraftKey, error) {
	var err error
	k.Repository, err = normalizeRepository(k.Repository)
	if err != nil || k.Number <= 0 || !shaPattern.MatchString(k.BaseSHA) || !shaPattern.MatchString(k.HeadSHA) {
		return k, errors.New("invalid draft comparison")
	}
	k.BaseRepository, err = normalizeRepository(k.BaseRepository)
	if err != nil {
		return k, err
	}
	k.HeadRepository, err = normalizeRepository(k.HeadRepository)
	return k, err
}
func draftArgs(k DraftKey) []any {
	return []any{k.Repository, k.Number, k.BaseSHA, k.HeadSHA, k.BaseRepository, k.HeadRepository}
}

const draftWhere = `repository=? AND pr_number=? AND base_sha=? AND head_sha=? AND base_repository=? AND head_repository=?`

func validateDraft(k DraftKey, d Draft) error {
	bad := errors.New("invalid review draft; original retained")
	if (d.Version != 1 && d.Version != 2 && d.Version != 3) || d.Event < 0 || d.Event > 2 || len(d.Pending) > source.MaxPendingReviewComments || !utf8.ValidString(d.Summary) {
		return bad
	}
	if d.Version < 3 && (d.SuggestionApply != nil || d.Composer != nil && d.Composer.Suggestion || d.Attempt == "suggestion") {
		return bad
	}
	switch d.Attempt {
	case "", "comment", "reply", "review", "suggestion":
	default:
		return bad
	}
	validEditor := func(e *DraftEditor, reply bool) bool {
		if e == nil {
			return true
		}
		t := e.Target
		if !reply && (t.StartLine != 0 || t.SubjectType == "file") && t.CommitID != k.HeadSHA {
			return false
		}
		repo, err := normalizeRepository(t.Identity.Repository)
		return err == nil && repo == k.Repository && t.Identity.Number == k.Number && (t.CommitID == k.HeadSHA || e.CommitSHA == t.CommitID && shaPattern.MatchString(e.CommitSHA) || reply && shaPattern.MatchString(t.CommitID)) && source.ValidateReviewCommentTarget(t) == nil && utf8.ValidString(e.Body) && (!reply || e.CommentID > 0 && e.ReplyToID > 0)
	}
	if r := d.Reply; r != nil && r.RootAnchor != nil {
		raw := r.RootAnchor
		display := r.Target
		normalized := *raw
		normalized.CommitID = display.CommitID
		if normalized != display || source.ValidateReviewCommentTarget(*raw) != nil {
			return bad
		}
	}
	if d.SuggestionApply != nil && (source.ValidateSuggestionApplication(*d.SuggestionApply) != nil || DraftKeyFor(d.SuggestionApply.Metadata) != k) {
		return bad
	}
	if !validEditor(d.Composer, false) || !validEditor(d.Reply, true) {
		return bad
	}
	for _, c := range d.Pending {
		if !validEditor(&DraftEditor{Target: c.Target, Body: c.Body}, false) || c.Body == "" {
			return bad
		}
	}
	if d.Composer != nil && (d.Composer.PendingIndex < -1 || d.Composer.PendingIndex >= len(d.Pending)) {
		return bad
	}
	if d.Attempt != "" && (d.Attempted == nil || d.Attempted.Kind != d.Attempt) {
		return bad
	}
	if a := d.Attempted; a != nil {
		if a.Kind != d.Attempt || a.Kind == "" {
			return bad
		}
		switch a.Kind {
		case "suggestion":
			if a.Application == nil || a.Comment != nil || a.Review != nil || source.ValidateSuggestionApplication(*a.Application) != nil || DraftKeyFor(a.Application.Metadata) != k {
				return bad
			}
		case "comment", "reply":
			if a.Comment == nil || a.Review != nil || a.ParentID < 0 || a.Kind == "reply" && a.ParentID <= 0 || !validEditor(&DraftEditor{Target: a.Comment.Target, Body: a.Comment.Body, CommitSHA: a.Comment.Target.CommitID, CommentID: a.ParentID, ReplyToID: a.ParentID}, a.Kind == "reply") {
				return bad
			}
		case "review":
			if a.Review == nil || a.Comment != nil || source.ValidatePullRequestReview(*a.Review) != nil {
				return bad
			}
			repo, err := normalizeRepository(a.Review.Identity.Repository)
			if err != nil || repo != k.Repository || a.Review.Identity.Number != k.Number || a.Review.CommitID != k.HeadSHA {
				return bad
			}
		}
	}
	if d.Attempt == "comment" && d.Composer == nil || d.Attempt == "reply" && d.Reply == nil {
		return bad
	}
	return nil
}

// ensureDrafts adds an optional mutable table without changing the immutable
// snapshot schema or its ownership marker. Older stores upgrade transactionally.
func (s *Store) ensureDrafts(ctx context.Context) error {
	db, err := s.db.SQL()
	if err != nil {
		return err
	}
	if s.db.ReadOnly() {
		return storage.ErrReadOnly
	}
	_, err = db.ExecContext(ctx, draftSchema)
	return storage.Classify(err)
}
func (s *Store) LoadDraft(ctx context.Context, key DraftKey) (Draft, error) {
	ctx, cancel := storage.OperationContext(ctx)
	defer cancel()
	key, err := normalizeDraftKey(key)
	if err != nil {
		return Draft{}, err
	}
	db, err := s.db.SQL()
	if err != nil {
		return Draft{}, err
	}
	if !s.db.ReadOnly() {
		if err = s.ensureDrafts(ctx); err != nil {
			return Draft{}, err
		}
	} else {
		var count int
		if err = db.QueryRowContext(ctx, `SELECT count(*) FROM sqlite_master WHERE type='table' AND name='review_drafts'`).Scan(&count); err != nil {
			return Draft{}, storage.Classify(err)
		}
		if count == 0 {
			return Draft{Version: 1}, nil
		}
	}
	var payload []byte
	var generation uint64
	err = db.QueryRowContext(ctx, `SELECT generation,payload FROM review_drafts WHERE `+draftWhere, draftArgs(key)...).Scan(&generation, &payload)
	if errors.Is(err, sql.ErrNoRows) {
		return Draft{Version: 1}, nil
	}
	if err != nil {
		return Draft{}, storage.Classify(err)
	}
	var d Draft
	if len(payload) > 1<<20 || json.Unmarshal(payload, &d) != nil || validateDraft(key, d) != nil {
		return Draft{}, errors.New("invalid review draft; original retained")
	}
	d.Generation = generation
	return d, nil
}
func (s *Store) SaveDraft(ctx context.Context, key DraftKey, expected uint64, d Draft) (Draft, error) {
	ctx, cancel := storage.OperationContext(ctx)
	defer cancel()
	key, err := normalizeDraftKey(key)
	if err != nil {
		return Draft{}, err
	}
	d.Version = 2
	if d.SuggestionApply != nil || d.Composer != nil && d.Composer.Suggestion || d.Attempt == "suggestion" {
		d.Version = 3
	}
	d.Generation = 0
	if expected >= math.MaxInt64 || validateDraft(key, d) != nil {
		return Draft{}, errors.New("invalid review draft update")
	}
	payload, err := json.Marshal(d)
	if err != nil || len(payload) > 1<<20 {
		return Draft{}, errors.New("review draft exceeds private storage limit")
	}
	if err = s.ensureDrafts(ctx); err != nil {
		return Draft{}, err
	}
	db, err := s.db.SQL()
	if err != nil {
		return Draft{}, err
	}
	args := draftArgs(key)
	var result sql.Result
	if expected == 0 {
		result, err = db.ExecContext(ctx, `INSERT INTO review_drafts(repository,pr_number,base_sha,head_sha,base_repository,head_repository,generation,payload) VALUES(?,?,?,?,?,?,1,?) ON CONFLICT DO NOTHING`, append(args, payload)...)
	} else {
		values := []any{payload}
		values = append(values, args...)
		values = append(values, expected)
		result, err = db.ExecContext(ctx, `UPDATE review_drafts SET payload=?,generation=generation+1 WHERE `+draftWhere+` AND generation=?`, values...)
	}
	if err != nil {
		return Draft{}, storage.Classify(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		return Draft{}, storage.Classify(err)
	}
	if n != 1 {
		return Draft{}, ErrStateConflict
	}
	d.Generation = expected + 1
	return d, nil
}
