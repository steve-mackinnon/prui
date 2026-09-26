package tui

import (
	"context"
	"fmt"
	"strings"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/ansi"
	"pr-review/internal/guide"
	"pr-review/internal/review"
)

type guidePreparationResult struct {
	Target      int
	SessionID   string
	Generation  uint64
	Preparation *guide.Preparation
	Err         error
}

type preparedGuideResult struct {
	Target     int
	SessionID  string
	Generation uint64
	Session    *review.Session
	Err        error
}

func (m *Model) SetGuidePreparation(prepare func(context.Context, *review.Session) (*guide.Preparation, error)) {
	m.prepareGuide = prepare
}
func (m *Model) SetPreparedGuideLifecycle(generate func(context.Context, *review.Session, *guide.Preparation) (*review.Session, error)) {
	m.generatePreparedGuide = generate
}

func (m *Model) startGuidePreparation() tea.Cmd {
	if m.Session == nil {
		return nil
	}
	m.discardGuidePreparation()
	m.guidePreparationGeneration++
	generation, target, s, prepare := m.guidePreparationGeneration, m.activeTab, *m.Session, m.prepareGuide
	m.notice = "Preparing pinned source locally; nothing is uploaded..."
	ctx := m.beginAction()
	m.guidePreparationPending = true
	return m.start(func() tea.Msg {
		p, err := prepare(ctx, &s)
		if ctx.Err() != nil {
			err = ctx.Err()
			p = nil
		}
		return guidePreparationResult{Target: target, SessionID: s.ID, Generation: generation, Preparation: p, Err: err}
	})
}

func (m *Model) discardGuidePreparation() {
	if m.guidePreparationPending {
		m.cancelCurrentAction()
		m.guidePreparationPending = false
		m.Busy = false
	}
	m.guidePreparationGeneration++
	m.guidePreparation = nil
	m.guideInspect = ""
	if m.top() == pageGuideConsent {
		m.pop()
	}
}

func (m *Model) preparedGuideConsentKey(k string) tea.Cmd {
	p := m.guidePreparation
	files := p.Files()
	m.guideSourceIndex = max(0, min(m.guideSourceIndex, len(files)-1))
	if m.guideInspect != "" {
		switch k {
		case "esc":
			m.guideInspect = ""
			m.guideSourceScroll = 0
		case "down", "j":
			m.guideSourceScroll++
		case "up", "k":
			m.guideSourceScroll = max(0, m.guideSourceScroll-1)
		case "pgdown":
			m.guideSourceScroll += max(1, m.Height-8)
		case "pgup":
			m.guideSourceScroll = max(0, m.guideSourceScroll-max(1, m.Height-8))
		case "home":
			m.guideSourceScroll = 0
		case "end":
			m.guideSourceScroll = 1 << 30
		}
		return nil
	}
	switch k {
	case "esc":
		m.discardGuidePreparation()
	case "down", "j":
		m.guideSourceIndex = min(max(0, len(files)-1), m.guideSourceIndex+1)
	case "up", "k":
		m.guideSourceIndex = max(0, m.guideSourceIndex-1)
	case "v":
		if len(files) > 0 {
			m.guideInspect = "source"
			m.guideSourceScroll = 0
		}
	case "o":
		m.guideInspect = "omissions"
		m.guideSourceScroll = 0
	case "d":
		m.guideInspect = "diff"
		m.guideSourceScroll = 0
	case "x":
		if len(files) > 0 {
			m.ActionError = p.Exclude(string(files[m.guideSourceIndex].Evidence.Path))
			m.guideSourceIndex = max(0, min(m.guideSourceIndex, len(p.Files())-1))
		}
	case "enter":
		if m.generatePreparedGuide == nil || m.Session == nil {
			return nil
		}
		p.Confirm()
		m.guidePreparation = nil
		m.pop()
		generation, target, s, generate := m.guidePreparationGeneration, m.activeTab, *m.Session, m.generatePreparedGuide
		m.notice = "Sending approved pinned source to " + Escape(m.guideRecipient()) + "..."
		ctx := m.beginAction()
		m.guidePreparationPending = true
		return m.start(func() tea.Msg {
			derived, err := generate(ctx, &s, p)
			if ctx.Err() != nil {
				err = ctx.Err()
			}
			return preparedGuideResult{Target: target, SessionID: s.ID, Generation: generation, Session: derived, Err: err}
		})
	}
	return nil
}

func (m *Model) preparedGuideConsentView() string {
	p := m.guidePreparation
	files := p.Files()
	in := p.Preview()
	if m.guideInspect != "" {
		title, body := "Initial patches and evidence to send", ""
		if m.guideInspect == "source" && len(files) > 0 {
			f := files[max(0, min(m.guideSourceIndex, len(files)-1))]
			title = fmt.Sprintf("%s — %s", f.Evidence.Path, f.Revision)
			body = fmt.Sprintf("Pinned commit: %s\nBlob: %s\n\n%s", f.Evidence.CommitSHA, f.Evidence.BlobID, f.Evidence.Excerpt)
		} else if m.guideInspect == "omissions" {
			title = "Local omissions and withheld source (not uploaded)"
			var b strings.Builder
			for _, o := range in.Omissions {
				fmt.Fprintf(&b, "Omitted %s: %s\n", o.Path, o.Reason)
			}
			for _, o := range in.Withheld {
				fmt.Fprintf(&b, "Withheld %s: %s\n", o.Path, o.Reason)
			}
			if b.Len() == 0 {
				b.WriteString("No source omissions recorded.")
			}
			body = b.String()
		} else {
			var b strings.Builder
			for _, u := range in.Units {
				fmt.Fprintf(&b, "%s\n%s\n", u.Path, u.Patch)
			}
			for _, e := range in.Evidence {
				fmt.Fprintf(&b, "%s\n%s\n", e.Path, e.Excerpt)
			}
			body = b.String()
		}
		var lines []string
		for _, line := range strings.Split(body, "\n") {
			lines = append(lines, strings.Split(ansi.Wrap(Escape(line), max(1, m.Width), ""), "\n")...)
		}
		height := max(1, m.Height-5)
		m.guideSourceScroll = max(0, min(m.guideSourceScroll, max(0, len(lines)-height)))
		return ansi.Truncate(Escape(title), max(1, m.Width), "…") + "\n\n" + strings.Join(lines[m.guideSourceScroll:min(len(lines), m.guideSourceScroll+height)], "\n") + "\n\nup/down · pgup/pgdown: scroll | esc: back (does not send)"
	}
	recipient := m.guideRecipient()
	if p.Recipient != "" {
		recipient = p.Recipient
	}
	header := []string{"Generate guide? Source approval for this attempt", "Recipient: " + Escape(recipient) + " | Model: " + Escape(p.Model), "Requests send your API key to this recipient and may upload patches and selected file excerpts. Working files are never searched.", "Credential filtering can miss secrets. Requests use store:false; your API key is not persisted.", "Up to 4 requests, 8 tool calls, 256 KiB evidence, 60 seconds; selected excerpts from approved files may be uploaded."}
	if p.UnavailableReason != "" {
		header = append(header, Escape(p.UnavailableReason))
	}
	header = append(header, fmt.Sprintf("Eligible files: %d | Omitted: %d | Withheld: %d | Initial units: %d | Byte limit: %d", len(files), len(in.Omissions), len(in.Withheld), len(in.Units), in.Limits.Bytes))
	if in.Search != nil && in.Search.Incomplete {
		header = append(header, "Search corpus incomplete: some source was withheld or exceeded local limits.")
	}
	var wrapped []string
	for _, line := range header {
		wrapped = append(wrapped, strings.Split(ansi.Wrap(line, max(1, m.Width), ""), "\n")...)
	}
	header = wrapped
	rows := []string{}
	for _, f := range files {
		rows = append(rows, Escape(fmt.Sprintf("%s [%s] (%d bytes)", f.Evidence.Path, f.Revision, len(f.Evidence.Excerpt))))
	}
	footer := strings.Split(ansi.Wrap("enter: approve and send | esc: cancel | v: inspect | d: patches | o: omissions | x: exclude | up/down: select", max(1, m.Width), ""), "\n")
	if m.ActionError != nil {
		header = append(header, Escape(m.ActionError.Error()))
	}
	header = header[:min(len(header), max(0, m.Height-len(footer)-1))]
	count := max(1, m.Height-len(header)-len(footer))
	start := max(0, min(m.guideSourceIndex-count/2, len(rows)-count))
	lines := append([]string(nil), header...)
	if len(rows) == 0 {
		lines = append(lines, "No additional source files eligible.")
	}
	for i := start; i < min(len(rows), start+count); i++ {
		lines = append(lines, clip(selectionMarker(i == m.guideSourceIndex)+rows[i], m.Width))
	}
	lines = append(lines, footer...)
	return strings.Join(lines, "\n")
}
