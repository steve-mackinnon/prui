package tui

import (
	"context"
	"fmt"
	"maps"
	"sort"
	"strings"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"prui/internal/review"
	"prui/internal/source"
)

// DiscussionSnapshot separates authoritative thread status from comparison
// freshness. Historical ownership remains readable when current pins differ.
type DiscussionSnapshot struct {
	Snapshot        source.DiscussionSnapshot
	CurrentVerified bool
	Reason          string
}
type DiscussionReader func(context.Context, *review.Session) (DiscussionSnapshot, error)
type DiscussionResult struct {
	Target     int
	Session    *review.Session
	Generation uint64
	Snapshot   DiscussionSnapshot
	Err        error
}
type discussionCount struct{ total, outdated int }

type discussionState struct {
	anchorIndex        map[source.ReviewCommentTarget][]source.Discussion
	counts             map[string]discussionCount
	indexGeneration    uint64
	cancel             context.CancelFunc
	snapshot           DiscussionSnapshot
	loaded             bool
	generation         uint64
	confirmedPublished map[string]confirmedPublished
	published          *publishedEditor
	editor             *generalCommentEditor
	selectedID         string
	confirmed          map[int64]bool
	confirmedEvents    map[string]source.ConversationEvent
	selected, scroll   int
	detail             bool
	notice             string
	returnCommit       *commitState
	returnView         reviewView
}

func (m *Model) SetDiscussionReader(read DiscussionReader) { m.readDiscussions = read }
func (m *Model) discussionGeneration() uint64              { return m.discussions.generation }
func (m *Model) refreshDiscussions() tea.Cmd {
	if m.readDiscussions == nil || m.Session == nil {
		return nil
	}
	if m.discussions.cancel != nil {
		m.discussions.cancel()
	}
	ctx, cancel := context.WithCancel(m.ctx)
	m.discussions.cancel = cancel
	m.discussions.generation++
	target, s, generation, read := m.activeTab, m.Session, m.discussions.generation, m.readDiscussions
	m.discussions.notice = "Refreshing discussions..."
	return func() tea.Msg {
		snapshot, err := read(ctx, s)
		return DiscussionResult{Target: target, Session: s, Generation: generation, Snapshot: snapshot, Err: err}
	}
}
func (m *Model) applyDiscussionResult(v DiscussionResult) {
	state := m.reviewStateForTarget(v.Target)
	if state == nil || state.Session != v.Session || state.discussions.generation != v.Generation {
		return
	}
	d := &state.discussions
	if v.Err != nil {
		label := "Discussions stale: "
		if !d.loaded {
			label = "Discussions unavailable: "
		}
		d.notice = label + Escape(v.Err.Error())
		return
	}
	if v.Snapshot.Snapshot.Complete {
		d.confirmed = nil
	} else {
		seen := map[int64]bool{}
		for _, thread := range v.Snapshot.Snapshot.Threads {
			for _, c := range thread.Comments {
				seen[c.ID] = true
			}
		}
		// Retain only missing confirmed identities in canonical threads.
		for cid := range d.confirmed {
			if !seen[cid] {
				retainPublishedComments(d, &v.Snapshot, "", cid)
			}
		}
	}

	if !v.Snapshot.Snapshot.Complete && v.Snapshot.Snapshot.Timeline {
		incoming := map[string]bool{}
		for _, event := range v.Snapshot.Snapshot.Events {
			incoming[event.ID] = true
		}
		for _, event := range d.snapshot.Snapshot.Events {
			if !incoming[event.ID] {
				event.Retained = true
				v.Snapshot.Snapshot.Events = append(v.Snapshot.Snapshot.Events, event)
			}
		}
		// Keep the selected inline activity readable without treating its old current
		// anchor as newly verified. Other omitted thread counts remain partial.
		selectedFound := false
		for _, entry := range discussionEntries(v.Snapshot.Snapshot) {
			if entry.ID == d.selectedID {
				selectedFound = true
				break
			}
		}
		if !selectedFound {
			for _, thread := range d.snapshot.Snapshot.Threads {
				for _, comment := range thread.Comments {
					if fmt.Sprintf("inline:%d", comment.ID) == d.selectedID {
						retainPublishedComments(d, &v.Snapshot, "", comment.ID)
						break
					}
				}
			}
		}
	}
	seenEvents := map[string]bool{}
	for _, event := range v.Snapshot.Snapshot.Events {
		seenEvents[event.ID] = true
		if !event.Retained {
			delete(d.confirmedEvents, event.ID)
		}
	}
	for id, event := range d.confirmedEvents {
		if !seenEvents[id] {
			v.Snapshot.Snapshot.Events = append(v.Snapshot.Snapshot.Events, event)
			v.Snapshot.Snapshot.Complete = false
			v.Snapshot.Snapshot.Reason = "Confirmed PR comment not yet observed on refresh"
		}
	}
	reconcilePublished(d, v.Snapshot)
	retainPublished(d, &v.Snapshot)
	d.snapshot, d.loaded, d.notice = v.Snapshot, true, ""
	if d.editor != nil {
		if v.Snapshot.Snapshot.Complete && v.Snapshot.CurrentVerified {
			if d.editor.uncertain {
				for _, event := range v.Snapshot.Snapshot.Events {
					if event.Kind == "PR comment" && event.Body == d.editor.attemptedBody && !d.editor.attemptedIDs[event.ID] {
						d.editor.matched = true
						d.notice = "Matching attempted PR comment found; inspect it before starting another comment"
						break
					}
				}
				d.editor.uncertain = false
			}
		}
	}
	d.anchorIndex = nil
	state.Comments = discussionCurrentComments(v.Snapshot, state.Session)
	state.commit.cache = commitRenderCache{}
	d.selected = 0
	entries := discussionEntries(d.snapshot.Snapshot)
	for i, thread := range entries {
		if thread.ID == d.selectedID {
			d.selected = i
			break
		}
	}
	if len(entries) > 0 {
		d.selectedID = entries[d.selected].ID
	} else {
		d.selectedID = ""
		d.detail = false
	}
	if v.Target == m.activeTab {
		m.ensureCommitEditorVisible()
	}
}
func discussionStatus(d source.Discussion) string {
	if d.Kind != "" {
		label := d.Kind
		if d.Retained {
			label += " · stale retained"
		}
		if d.Decision != "" {
			label += " · " + d.Decision
		}
		if !d.CreatedAt.IsZero() {
			label += " · " + d.CreatedAt.UTC().Format(time.RFC3339)
		} else {
			label += " · timestamp unavailable"
		}
		if d.Kind != "Inline comment" && d.Kind != "Inline reply" {
			return label
		}
		d.Kind = ""
		return label + " · " + discussionStatus(d)
	}
	labels := []string{}
	if d.Outdated != nil && *d.Outdated {
		labels = append(labels, "Outdated on current PR")
	}
	if d.Resolved != nil && *d.Resolved {
		labels = append(labels, "Resolved")
	}
	if d.Outdated == nil || d.Resolved == nil {
		labels = append(labels, "Status unknown")
	}
	return strings.Join(labels, " · ")
}

// indexDiscussionAnchors avoids scanning every thread for every captured patch
// line. The event-loop owner invalidates it on a published snapshot or mutation.
func (m *Model) indexDiscussionAnchors() {
	d := &m.discussions
	if d.anchorIndex != nil && d.indexGeneration == d.generation {
		return
	}
	d.anchorIndex = map[source.ReviewCommentTarget][]source.Discussion{}
	d.counts = map[string]discussionCount{}
	for _, thread := range d.snapshot.Snapshot.Threads {
		count := d.counts[thread.OriginalCommitID]
		count.total++
		if thread.Outdated != nil && *thread.Outdated {
			count.outdated++
		}
		d.counts[thread.OriginalCommitID] = count
		if thread.OriginalAnchor != nil && thread.OriginalAnchor.CommitID == thread.OriginalCommitID {
			target := *thread.OriginalAnchor
			d.anchorIndex[target] = append(d.anchorIndex[target], thread)
		}
	}
	d.indexGeneration = d.generation
}

func (m *Model) commitDiscussionCount(sha string) string {
	d := m.discussions
	if !d.loaded {
		return "Threads unavailable"
	}
	m.indexDiscussionAnchors()
	count := m.discussions.counts[sha]
	total, outdated := count.total, count.outdated
	label := "threads"
	if !d.snapshot.Snapshot.Complete {
		label = "loaded threads"
	}
	text := fmt.Sprintf("%d %s", total, label)
	if outdated > 0 {
		text += fmt.Sprintf(" · %d outdated", outdated)
	}
	return text
}
func (m *Model) commitDiscussionLines(target source.ReviewCommentTarget) []diffLine {
	var lines []diffLine
	m.indexDiscussionAnchors()
	for _, thread := range m.discussions.anchorIndex[target] {
		if label := discussionStatus(thread); label != "" {
			lines = append(lines, diffLine{styledLine: styledLine{Class: classMetadata, Text: "  " + label}})
		}
		for i, c := range thread.Comments {
			indent := 0
			if i > 0 {
				indent = 4
			}
			cards := m.reviewCommentLinesAt(c, indent)
			for j := range cards {
				cards[j].commentID = 0
			}
			lines = append(lines, cards...)
		}
	}
	return lines
}
func (m *Model) openDiscussions() {
	m.discussions.detail = false
	m.discussions.scroll = 0
	m.push(pageDiscussions)
}
func (m *Model) discussionKey(key string) tea.Cmd {
	d := &m.discussions
	threads := discussionEntries(d.snapshot.Snapshot)
	switch key {
	case "alt+up", "alt+down":
		delta := 1
		if key == "alt+up" {
			delta = -1
		}
		m.nextUnresolved(delta)
		return nil
	case "e", "z":
		if len(threads) == 0 || !d.detail {
			return nil
		}
		t := threads[d.selected]
		cid := int64(0)
		if len(t.Comments) > 0 {
			cid = t.Comments[0].ID
		}
		event := ""
		if t.Kind == "PR comment" {
			event = t.ID
		}
		if key == "e" && m.Viewer == "" {
			d.notice = "Load authenticated viewer, then press e again"
			return m.refreshViewer()
		}
		if !m.openPublished(cid, event, key == "z") {
			d.notice = "Published action unavailable or permission denied"
		}
		return nil
	case "n", "r":
		if m.submitGeneralComment == nil {
			d.notice = "PR comments unavailable · offline or unsupported"
			return nil
		}
		if key == "r" && (!d.detail || len(threads) == 0 || threads[d.selected].Kind != "PR comment") {
			d.notice = "General replies require a PR comment; inline replies use diff comment actions"
			return nil
		}
		d.editor = &generalCommentEditor{}
		if key == "r" {
			d.editor.replyTo = threads[d.selected].ID
			d.editor.draft = "@" + threads[d.selected].Comments[0].Author + " "
			d.editor.cursor = len([]rune(d.editor.draft))
		}
		return nil
	case "c":
		return m.refreshDiscussions()
	case "esc":
		if d.detail {
			d.detail = false
			d.scroll = 0
		} else {
			m.pop()
		}
		return nil
	case "enter":
		if len(threads) > 0 {
			d.detail = true
			d.scroll = 0
		}
		return nil
	case "o":
		if !d.detail || len(threads) == 0 {
			return nil
		}
		t := threads[d.selected]
		saved := m.commit
		saved.offsets = maps.Clone(saved.offsets)
		saved.cursors = maps.Clone(saved.cursors)
		previous := m.ContextView
		if m.viewOriginalDiscussion(t.OriginalAnchor, t.OriginalCommitID) {
			d.returnCommit = &saved
			d.returnView = previous
			m.pop()
		}
		return nil
	case "j", "down":
		if d.detail {
			d.scroll++
		} else {
			d.selected = min(len(threads)-1, d.selected+1)
		}
	case "k", "up":
		if d.detail {
			d.scroll = max(0, d.scroll-1)
		} else {
			d.selected = max(0, d.selected-1)
		}
	case "pgdown", "d":
		if d.detail {
			d.scroll += max(1, m.Height-5)
		}
	case "pgup", "u":
		if d.detail {
			d.scroll = max(0, d.scroll-max(1, m.Height-5))
		}
	}
	if len(threads) > 0 {
		d.selected = max(0, min(len(threads)-1, d.selected))
		d.selectedID = threads[d.selected].ID
	}
	return nil
}
func (m *Model) restoreDiscussionContext() bool {
	d := &m.discussions
	if d.returnCommit == nil {
		return false
	}
	m.commit = *d.returnCommit
	m.ContextView = d.returnView
	d.returnCommit = nil
	d.detail = true
	m.push(pageDiscussions)
	return true
}
func (m *Model) discussionsView() string {
	d := &m.discussions
	lines := []string{"Discussions"}
	if d.snapshot.Snapshot.Timeline {
		lines[0] = "PR conversation · chronological live activity"
	}
	if d.editor != nil {
		return m.generalCommentView()
	}
	if d.notice != "" {
		lines = append(lines, d.notice)
	}
	if !d.loaded {
		label := "Discussions unavailable. c: refresh"
		if m.readDiscussions == nil {
			label = "Discussions unavailable · offline or unsupported"
		}
		lines = append(lines, label)
	} else {
		if !d.snapshot.CurrentVerified {
			lines = append(lines, "Discussions from live PR · snapshot differs", Escape(d.snapshot.Reason))
		}
		if !d.snapshot.Snapshot.Complete {
			lines = append(lines, func() string {
				if d.snapshot.Snapshot.Timeline {
					return "Loaded activity · partial/incomplete"
				}
				return "Loaded threads · incomplete"
			}(), Escape(d.snapshot.Snapshot.Reason))
		}
		threads := discussionEntries(d.snapshot.Snapshot)
		switch {
		case len(threads) == 0:
			if d.snapshot.Snapshot.Complete {
				lines = append(lines, "No discussions.")
			} else {
				label := "No threads loaded."
				if d.snapshot.Snapshot.Timeline {
					label = "No activity loaded."
				}
				lines = append(lines, label)
			}
		case d.detail:
			t := threads[max(0, min(d.selected, len(threads)-1))]
			body := []string{discussionStatus(t)}
			general := t.Kind == "PR comment" || t.Kind == "Review"
			if t.CurrentAnchor != nil {
				body = append(body, "Current target · "+commentTargetLabel(*t.CurrentAnchor))
			}
			if t.OriginalAnchor != nil {
				body = append(body, "Original target · "+commentTargetLabel(*t.OriginalAnchor))
			}
			captured := false
			for _, e := range m.commitEntries() {
				if e.SHA == t.OriginalCommitID {
					captured = true
				}
			}
			if !general && t.OriginalCommitID == "" {
				body = append(body, "Original commit unavailable")
			} else if !general && !captured {
				body = append(body, "Original commit not captured")
			}
			if !general && (t.OriginalAnchor == nil || !m.discussionOriginalAvailable(t)) {
				body = append(body, "Original context unavailable")
			} else if !general {
				body = append(body, fmt.Sprintf("%s · %s:%d · %s", Escape(shortCommitSHA(t.OriginalCommitID)), Escape(t.OriginalAnchor.Path), t.OriginalAnchor.Line, Escape(t.OriginalAnchor.Side)))
			}
			for _, c := range t.Comments {
				body = append(body, "@"+Escape(c.Author), Escape(c.Body), "")
			}
			if t.DiffHunk != "" {
				body = append(body, "Historical snippet", Escape(t.DiffHunk))
			}
			if t.URL != "" {
				body = append(body, Escape(t.URL))
			}
			body = strings.Split(ansi.Wrap(strings.Join(body, "\n"), max(1, m.Width), ""), "\n")
			height := max(1, m.Height-len(lines)-2)
			d.scroll = min(d.scroll, max(0, len(body)-height))
			lines = append(lines, body[d.scroll:min(len(body), d.scroll+height)]...)
		default:
			height := max(1, m.Height-len(lines)-2)
			start := max(0, d.selected-height+1)
			for i := start; i < min(len(threads), start+height); i++ {
				t := threads[i]
				label := Escape(t.ID)
				if len(t.Comments) > 0 {
					label = "@" + Escape(t.Comments[0].Author) + " · " + Escape(strings.ReplaceAll(t.Comments[0].Body, "\n", " "))
				}
				marker := "  "
				if i == d.selected {
					marker = "> "
				}
				lines = append(lines, clip(marker+discussionStatus(t)+" · "+label, m.Width))
			}
		}
	}
	footer := "j/k: select · enter: detail · c: refresh · esc: back"
	if d.snapshot.Snapshot.Timeline {
		footer = "j/k: select · enter: detail · n: comment · c: refresh · esc: back"
	}
	if d.detail {
		footer = "j/k: scroll · e: edit · z: resolve/reopen · o: original · c: refresh · esc: back"
		if d.snapshot.Snapshot.Timeline {
			footer = "e: edit · z: resolve/reopen · r: general reply · o: original · c: refresh · esc: back"
		}
	}
	lines = append(lines, clip(footer, m.Width))
	return strings.Join(lines, "\n")
}

func discussionCurrentComments(snapshot DiscussionSnapshot, s *review.Session) []source.ReviewComment {
	if !snapshot.CurrentVerified {
		return nil
	}
	var comments []source.ReviewComment
	for _, thread := range snapshot.Snapshot.Threads {
		if thread.CurrentAnchor == nil || (thread.Outdated != nil && *thread.Outdated) {
			continue
		}
		for _, c := range thread.Comments {
			if c.Retained || thread.Retained {
				continue
			}
			associated := *thread.CurrentAnchor
			c.CurrentAnchor = &associated
			c.Target = *thread.CurrentAnchor
			c.Target.CommitID = s.Inventory.Comparison.Metadata.HeadSHA
			comments = append(comments, c)
		}
	}
	return commentOverlay(comments, s)
}

// insertCommentDiscussion preserves a confirmed creation before the next read.
func insertCommentDiscussion(state *reviewTabState, comment source.ReviewComment) {
	if state == nil {
		return
	}
	for _, thread := range state.discussions.snapshot.Snapshot.Threads {
		for _, existing := range thread.Comments {
			if existing.ID == comment.ID {
				return
			}
		}
	}
	anchor := comment.Target
	if comment.OriginalAnchor != nil {
		anchor = *comment.OriginalAnchor
	}
	thread := source.Discussion{ID: fmt.Sprintf("comment:%d", comment.ID), OriginalCommitID: anchor.CommitID, OriginalAnchor: &anchor, CurrentAnchor: discussionCreatedCurrent(comment), DiffHunk: comment.DiffHunk, URL: comment.URL, Comments: []source.ReviewComment{comment}}
	if state.discussions.confirmed == nil {
		state.discussions.confirmed = map[int64]bool{}
	}
	state.discussions.confirmed[comment.ID] = true
	state.discussions.snapshot.Snapshot.Threads = append(state.discussions.snapshot.Snapshot.Threads, thread)
	state.discussions.loaded = true
	state.discussions.generation++
	state.commit.cache = commitRenderCache{}
}

func updateDiscussionAction(state *reviewTabState, result CommentActionResult) {
	if state == nil || result.Err != nil {
		return
	}
	threads := state.discussions.snapshot.Snapshot.Threads
	for i := 0; i < len(threads); i++ {
		for j, c := range threads[i].Comments {
			if c.ID != result.CommentID {
				continue
			}
			if result.Delete {
				delete(state.discussions.confirmed, result.CommentID)
				if j == 0 {
					threads = append(threads[:i], threads[i+1:]...)
				} else {
					threads[i].Comments = append(threads[i].Comments[:j], threads[i].Comments[j+1:]...)
				}
			} else if result.Reply.ID > 0 {
				threads[i].Comments = append(threads[i].Comments, result.Reply)
			}
			state.discussions.snapshot.Snapshot.Threads = threads
			state.discussions.generation++
			state.commit.cache = commitRenderCache{}
			return
		}
	}
}
func (m *Model) discussionListStart() (int, int) {
	header := 1
	if m.discussions.notice != "" {
		header++
	}
	if m.discussions.loaded && !m.discussions.snapshot.CurrentVerified {
		header += 2
	}
	if m.discussions.loaded && !m.discussions.snapshot.Snapshot.Complete {
		header += 2
	}
	height := max(1, m.Height-header-2)
	return header, max(0, m.discussions.selected-height+1)
}
func (m *Model) discussionMouseClick(y int) {
	if m.discussions.detail {
		return
	}
	header, start := m.discussionListStart()
	i := start + y - header
	if y >= header && y < m.Height-1 && i >= 0 && i < len(m.discussionEntries()) {
		m.discussions.selected = i
		m.discussions.selectedID = m.discussionEntries()[i].ID
	}
}

func (m *Model) discussionOriginalAvailable(t source.Discussion) bool {
	if t.OriginalAnchor == nil {
		return false
	}
	for _, e := range m.commitEntries() {
		if e.SHA != t.OriginalCommitID || e.Diff == nil {
			continue
		}
		rows := commitDiffRowsFor(e.Diff, m.Session.Inventory.Comparison.Metadata.Identity, e.SHA)
		for _, row := range rows {
			if row.target != nil && *row.target == *t.OriginalAnchor {
				return true
			}
		}
	}
	return false
}

func discussionCreatedCurrent(c source.ReviewComment) *source.ReviewCommentTarget {
	if source.ValidateReviewCommentTarget(c.Target) != nil {
		return nil
	}
	target := c.Target
	return &target
}

func (m *Model) cancelDiscussionReads() {
	if m.discussions.cancel != nil {
		m.discussions.cancel()
	}
	for _, tab := range m.tabs {
		if tab.review != nil && tab.review.discussions.cancel != nil {
			tab.review.discussions.cancel()
		}
	}
}

// Expand inline activity without changing the authoritative thread collection.
func discussionEntries(snapshot source.DiscussionSnapshot) []source.Discussion {
	if !snapshot.Timeline {
		return snapshot.Threads
	}
	entries := []source.Discussion{}
	seen := map[string]bool{}
	for _, thread := range snapshot.Threads {
		for _, comment := range thread.Comments {
			entry := thread
			entry.Retained = thread.Retained || comment.Retained
			entry.ID = fmt.Sprintf("inline:%d", comment.ID)
			if seen[entry.ID] {
				continue
			}
			seen[entry.ID] = true
			entry.Kind = "Inline comment"
			if comment.ParentID != 0 {
				entry.Kind = "Inline reply"
			}
			entry.CreatedAt = comment.CreatedAt
			entry.Comments = []source.ReviewComment{comment}
			entries = append(entries, entry)
		}
	}
	for _, event := range snapshot.Events {
		if seen[event.ID] {
			continue
		}
		seen[event.ID] = true
		entries = append(entries, source.Discussion{ID: event.ID, Retained: event.Retained, Kind: event.Kind, Decision: event.Decision, CreatedAt: event.CreatedAt, URL: event.URL, Comments: []source.ReviewComment{{Author: event.Author, Body: event.Body, CreatedAt: event.CreatedAt}}})
	}
	sort.SliceStable(entries, func(i, j int) bool {
		a, b := entries[i], entries[j]
		if a.CreatedAt.Equal(b.CreatedAt) {
			return a.ID < b.ID
		}
		return a.CreatedAt.Before(b.CreatedAt)
	})
	return entries
}
func (m *Model) discussionEntries() []source.Discussion {
	return discussionEntries(m.discussions.snapshot.Snapshot)
}
