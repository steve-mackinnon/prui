package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"pr-review/internal/review"
	"pr-review/internal/source"
	"pr-review/internal/tui"
)

const usage = "pr-review open <PR-URL-or-number> --repo <checkout> [--github-repo owner/repo] [--plain]"

type options struct {
	Identity source.Identity
	Checkout string
	Plain    bool
}

func parseOptions(args []string) (options, error) {
	var o options
	if len(args) < 2 || args[0] != "open" {
		return o, errors.New(usage)
	}
	f := flag.NewFlagSet("open", flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Checkout, "repo", "", "existing local checkout")
	var repository string
	f.StringVar(&repository, "github-repo", "", "explicit owner/repo for numbers")
	f.BoolVar(&o.Plain, "plain", false, "non-interactive escaped text, no pager")
	if e := f.Parse(args[2:]); e != nil {
		return o, e
	}
	if f.NArg() != 0 || o.Checkout == "" {
		return o, errors.New(usage)
	}
	var e error
	o.Identity, e = source.ParseIdentity(args[1], repository)
	return o, e
}
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
	r := source.NewRunner()
	limits := source.Defaults()
	// gh runs outside both the workspace and the reviewed checkout.
	dir, e := os.MkdirTemp("", "pr-review-gh-")
	if e != nil {
		fmt.Fprintln(os.Stderr, "cannot create private metadata workspace")
		return 1
	}
	defer os.RemoveAll(dir)
	gh, e := source.NewGH(r, limits, dir)
	if e != nil {
		fmt.Fprintln(os.Stderr, "gh executable required; install GitHub CLI and authenticate")
		return 1
	}
	load := func(c context.Context, notify func(string)) (*review.Session, error) {
		return review.Open(c, o.Checkout, o.Identity, gh, r, limits, notify)
	}
	if o.Plain || os.Getenv("TERM") == "dumb" || !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		fmt.Fprintln(os.Stderr, "Reading GitHub PR metadata; source stays local. Missing objects may be fetched.")
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
	started, done := make(chan struct{}), make(chan struct{})
	m := tui.New(ctx, func(c context.Context, n func(string)) (*review.Session, error) {
		close(started)
		defer close(done)
		return load(c, n)
	})
	p := tea.NewProgram(m, tea.WithContext(ctx))
	m.SetNotifier(func(s string) { p.Send(tui.Notice(s)) })
	_, e = p.Run()
	cancel()
	select {
	case <-started:
		<-done
	default:
	}
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
