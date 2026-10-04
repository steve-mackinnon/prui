// Package inbox persists mutable triage metadata independently of frozen source.
package inbox

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"time"

	_ "modernc.org/sqlite"
	"prui/internal/source"
)

type Cache struct{ db *sql.DB }

func Open(path string) (*Cache, error) {
	if !filepath.IsAbs(path) {
		return nil, errors.New("inbox cache requires an absolute path")
	}
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_RDWR, 0600)
	if err == nil {
		err = f.Close()
	} else if os.IsExist(err) {
		info, e := os.Lstat(path)
		err = e
		if e == nil && (!info.Mode().IsRegular() || info.Mode().Perm()&0077 != 0) {
			err = errors.New("inbox cache must be a private regular file")
		}
	}
	if err != nil {
		return nil, err
	}
	u := url.URL{Scheme: "file", Path: path}
	db, err := sql.Open("sqlite", u.String()+"?_pragma=busy_timeout(1000)")
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	var id, version int
	if err = db.QueryRowContext(ctx, "PRAGMA application_id").Scan(&id); err == nil {
		err = db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version)
	}
	if err == nil && (id != 0x5052494e || version != 1) {
		var tables int
		err = db.QueryRowContext(ctx, "SELECT count(*) FROM sqlite_master WHERE type='table'").Scan(&tables)
		if err == nil && (id != 0 || version != 0 || tables != 0) {
			err = errors.New("unsupported inbox cache; original retained")
		}
		if err == nil {
			_, err = db.ExecContext(ctx, `BEGIN IMMEDIATE; CREATE TABLE captures (viewer TEXT NOT NULL COLLATE NOCASE, key TEXT NOT NULL, observed TEXT NOT NULL, payload BLOB NOT NULL, digest TEXT NOT NULL, PRIMARY KEY(viewer,key)); CREATE TABLE reads (viewer TEXT NOT NULL COLLATE NOCASE, identity TEXT NOT NULL, digest TEXT NOT NULL, PRIMARY KEY(viewer,identity)); PRAGMA application_id=1347569998; PRAGMA user_version=1; COMMIT;`)
		}
	}
	if err != nil {
		_ = db.Close()
		return nil, errors.New("inbox cache unavailable or unsupported; original retained")
	}
	return &Cache{db}, nil
}
func (c *Cache) Close() error { return c.db.Close() }
func key(o source.InboxOptions) string {
	o.Activity = ""
	o.Account = ""
	b, _ := json.Marshal(o)
	return string(b)
}
func identity(i source.InboxItem) string {
	return i.PullRequest.Identity.Repository + "#" + strconv.Itoa(i.PullRequest.Identity.Number)
}
func digest(i source.InboxItem) string {
	// Activity excludes view-dependent account/team membership annotations.
	evidence := struct {
		UpdatedAt                          time.Time
		Head, State, Review, Title, Author string
		Draft                              bool
		Teams                              []string
	}{i.UpdatedAt, i.HeadSHA, i.State, i.ReviewDecision, i.PullRequest.Title, i.PullRequest.Author, i.Draft, i.Teams}
	b, _ := json.Marshal(evidence)
	sum := sha256.Sum256(b)
	return hex.EncodeToString(sum[:])
}
func (c *Cache) Save(ctx context.Context, o source.InboxOptions, r source.Inbox) error {
	if err := o.Validate(); err != nil {
		return err
	}
	if len(r.Items) > 2000 || r.ObservedAt.IsZero() || r.Viewer == "" {
		return errors.New("invalid inbox capture")
	}
	r.Cached = false
	b, err := json.Marshal(r)
	if err != nil {
		return err
	}
	if len(b) > 4<<20 {
		return errors.New("inbox cache budget exceeded")
	}
	// Later captures win; a slow refresh cannot overwrite newer provenance.
	_, err = c.db.ExecContext(ctx, `INSERT INTO captures(viewer,key,observed,payload,digest) VALUES(?,?,?,?,?) ON CONFLICT(viewer,key) DO UPDATE SET observed=excluded.observed,payload=excluded.payload,digest=excluded.digest WHERE excluded.observed>captures.observed`, r.Viewer, key(o), r.ObservedAt.UTC().Format("2006-01-02T15:04:05.000000000Z"), b, payloadDigest(b))
	return err
}
func (c *Cache) Load(ctx context.Context, o source.InboxOptions) (source.Inbox, error) {
	return c.LoadViewer(ctx, o, o.Account)
}
func (c *Cache) LoadViewer(ctx context.Context, o source.InboxOptions, viewer string) (source.Inbox, error) {

	if err := o.Validate(); err != nil {
		return source.Inbox{}, err
	}
	var b []byte
	var expected string
	query := "SELECT payload,digest FROM captures WHERE key=?"
	args := []any{key(o)}
	if viewer != "" {
		query += " AND viewer=?"
		args = append(args, viewer)
	}
	query += " ORDER BY observed DESC,viewer ASC LIMIT 1"
	err := c.db.QueryRowContext(ctx, query, args...).Scan(&b, &expected)
	if errors.Is(err, sql.ErrNoRows) {
		return source.Inbox{}, errors.New("no cached inbox for these filters; explicitly refresh online")
	}
	if err != nil {
		return source.Inbox{}, err
	}
	var r source.Inbox
	if payloadDigest(b) != expected || len(b) > 4<<20 || json.Unmarshal(b, &r) != nil || len(r.Items) > 2000 || r.Viewer == "" || r.ObservedAt.IsZero() {
		return source.Inbox{}, errors.New("invalid inbox cache; original retained")
	}
	for j := range r.Items {
		i := &r.Items[j]
		var d string
		err := c.db.QueryRowContext(ctx, "SELECT digest FROM reads WHERE viewer=? AND identity=?", r.Viewer, identity(*i)).Scan(&d)
		switch {
		case errors.Is(err, sql.ErrNoRows):
			i.Activity = "unknown"
		case err != nil:
			return source.Inbox{}, err
		case d == digest(*i):
			i.Activity = "read"
		default:
			i.Activity = "changed"
		}
	}
	r.Cached = true
	return Filter(r, o), nil
}
func (c *Cache) MarkRead(ctx context.Context, viewer string, i source.InboxItem) error {
	if viewer == "" || i.PullRequest.Identity.Number <= 0 {
		return errors.New("invalid mark-read identity")
	}
	_, err := c.db.ExecContext(ctx, "INSERT INTO reads(viewer,identity,digest) VALUES(?,?,?) ON CONFLICT(viewer,identity) DO UPDATE SET digest=excluded.digest", viewer, identity(i), digest(i))
	return err
}
func Filter(r source.Inbox, o source.InboxOptions) source.Inbox {
	items := make([]source.InboxItem, 0, len(r.Items))
	for _, i := range r.Items {
		if o.Activity != "" && o.Activity != "all" && i.Activity != o.Activity {
			continue
		}
		items = append(items, i)
	}
	r.Items = items
	return r
}
func Label(r source.Inbox) string {
	if r.ObservedAt.IsZero() {
		return "Inbox not captured · freshness/access unknown"
	}
	provenance := "refreshed"
	if r.Cached {
		provenance = "cached"
	}
	coverage := "complete accessible search"
	if !r.Complete {
		coverage = "incomplete"
	}
	return fmt.Sprintf("%s %s · %s · @%s · %d requests · private/access-limited repositories may be absent", provenance, r.ObservedAt.UTC().Format(time.RFC3339), coverage, r.Viewer, r.Requests)
}

func payloadDigest(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }
