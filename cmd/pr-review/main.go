package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"pr-review/internal/review"
	"pr-review/internal/session"
	"pr-review/internal/source"
	"pr-review/internal/tui"
)

func run(args []string) int {
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println(usage)
		return 0
	}
	o, e := parseOptions(args)
	if e != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
		return 1
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if o.Storage == "" {
		o.Storage, e = session.DefaultPath()
		if e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
			return 1
		}
	}
	if o.Checkout != "" {
		if e := outsideCheckout(o.Storage, o.Checkout); e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
			return 1
		}
	}
	store, e := session.Open(o.Storage)
	if e != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
		return 1
	}
	defer store.Close()
	fmt.Fprintln(os.Stderr, "Local session storage:", tui.Escape(store.Path()))
	if o.Command == "sessions" {
		return listSessions(store, os.Stdout)
	}
	if o.Command == "delete" {
		if e := store.Delete(o.SessionID); e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
			return 1
		}
		fmt.Println("Deleted local session", o.SessionID, "(not provider records or forensic disk traces).")
		return 0
	}
	r := source.NewRunner()
	limits := source.Defaults()
	app := application{store: store, runner: r, limits: limits}
	// gh runs outside both the workspace and the reviewed checkout.
	if !o.Offline {
		dir, err := os.MkdirTemp("", "pr-review-gh-")
		app.setupError = err
		if err == nil {
			defer os.RemoveAll(dir)
			app.gh, app.setupError = source.NewGH(r, limits, dir)
		}
	}
	if o.Command == "prs" {
		if o.Repository != "" {
			prs, err := app.listPullRequests(ctx, o.Repository)
			if err != nil {
				fmt.Fprintln(os.Stderr, tui.Escape(err.Error()))
				return 1
			}
			listPullRequests(os.Stdout, o.Repository, prs)
			return 0
		}
		if o.Plain || os.Getenv("TERM") == "dumb" || !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
			fmt.Fprintln(os.Stderr, "prs without owner/repo requires an interactive terminal; pass owner/repo to list directly")
			return 1
		}
		m := tui.NewPullRequestBrowser(ctx, store, app.listPullRequests, func(c context.Context, checkout string, id source.Identity, n func(string)) (*review.Session, error) {
			return app.open(c, checkout, id, n)
		})
		p := tea.NewProgram(m, tea.WithContext(ctx))
		m.SetNotifier(func(s string) { p.Send(tui.Notice(s)) })
		_, err := p.Run()
		cancel()
		m.Close()
		if err != nil || m.ActionError != nil && m.Session == nil {
			return 1
		}
		if m.Session != nil && !m.Session.Inventory.Complete {
			return 2
		}
		return 0
	}
	load := func(c context.Context, notify func(string)) (*review.Session, error) {
		return app.load(c, o, notify)
	}
	if o.Plain || os.Getenv("TERM") == "dumb" || !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		if o.Offline {
			fmt.Fprintln(os.Stderr, "Offline snapshot; no freshness check or network access.")
		} else {
			fmt.Fprintln(os.Stderr, "Reading GitHub PR metadata; source stays local. New comparisons may fetch missing objects.")
		}
		s, e := load(ctx, func(s string) { fmt.Fprintln(os.Stderr, tui.Escape(s)) })
		if e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
			if ctx.Err() != nil {
				return 130
			}
			return 1
		}
		if _, e = fmt.Fprint(os.Stdout, tui.Plain(s)); e != nil {
			return 1
		}
		if !s.Inventory.Complete {
			return 2
		}
		return 0
	}
	m := tui.New(ctx, load)
	var reader review.MetadataReader = &app
	if o.Offline {
		reader = nil
	}
	m.SetLifecycle(store, reader, func(c context.Context, old *review.Session, n func(string)) (*review.Session, error) {
		if o.Offline {
			return nil, errors.New("offline mode: resume without --offline to start a new comparison")
		}
		override := ""
		if old.ID == o.SessionID {
			override = o.Checkout
		}
		return app.fresh(c, old, override, n)
	})
	m.SetPullRequestLifecycle(app.listPullRequests, func(c context.Context, checkout string, id source.Identity, n func(string)) (*review.Session, error) {
		return app.open(c, checkout, id, n)
	})
	p := tea.NewProgram(m, tea.WithContext(ctx))
	m.SetNotifier(func(s string) { p.Send(tui.Notice(s)) })
	_, e = p.Run()
	cancel()
	m.Close()
	if e != nil {
		fmt.Fprintln(os.Stderr, "terminal review stopped; use --plain for non-interactive output")
		return 1
	}
	if m.Err != nil {
		if errors.Is(m.Err, context.Canceled) {
			return 130
		}
		return 1
	}
	if m.Session != nil && !m.Session.Inventory.Complete {
		return 2
	}
	return 0
}
func main() { os.Exit(run(os.Args[1:])) }
