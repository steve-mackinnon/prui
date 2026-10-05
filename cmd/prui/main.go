package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/x/term"
	"prui/internal/guideeval"
	"prui/internal/session"
	"prui/internal/source"
	"prui/internal/tui"
)

// Release builds set these from the Git tag and commit using linker flags.
var version = "dev"
var commit = "unknown"

func run(args []string) int {
	if len(args) == 1 && args[0] == "--version" {
		fmt.Printf("prui %s (commit %s)\n", version, commit)
		return 0
	}
	if len(args) == 1 && (args[0] == "--help" || args[0] == "-h") {
		fmt.Println(usage)
		return 0
	}
	o, e := parseOptions(args)
	if e != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
		return 1
	}
	if o.Command == "open" || o.Command == "verify" || o.Command == "current" {
		o.Checkout, e = checkoutFromWorkingDirectory()
		if e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
			return 1
		}
	}
	if o.Command == "current" && (o.Plain || os.Getenv("TERM") == "dumb" || !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd())) {
		fmt.Fprintln(os.Stderr, "prui without arguments requires an interactive terminal; use prs owner/repo --plain or open instead")
		return 1
	}
	if o.Command == "current" {
		o.Repository, e = source.RepositoryFromCheckout(o.Checkout)
		if e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape("current checkout has no supported GitHub origin: "+e.Error()))
			return 1
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	if o.Command == "verify" {
		return runVerify(ctx, o)
	}
	storagePath, e := session.DefaultPath()
	if e != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
		return 1
	}
	if o.Checkout != "" {
		if e := outsideCheckout(storagePath, o.Checkout); e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
			return 1
		}
	}
	var store *session.Store
	if o.Command == "eval-guides" {
		store, e = session.OpenReadOnly(storagePath)
	} else {
		store, e = session.Open(storagePath)
	}
	if e != nil {
		fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
		return 1
	}
	defer func() { _ = store.Close() }()
	fmt.Fprintln(os.Stderr, "Local session storage:", tui.Escape(store.Path()))
	if o.Command == "sessions" {
		return listSessions(store, os.Stdout)
	}
	if o.Command == "eval-guides" {
		if e := evalGuides(store, o.SessionID, os.Stdout); e != nil {
			fmt.Fprintln(os.Stderr, tui.Escape(e.Error()))
			return 1
		}
		return 0
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
	app := application{store: store, runner: r, limits: limits, cacheFullSource: o.CacheFullSource, offline: o.Offline}
	// gh runs outside both the workspace and the reviewed checkout.
	if !o.Offline {
		dir, err := os.MkdirTemp("", "prui-gh-")
		app.setupError = err
		if err == nil {
			defer func() { _ = os.RemoveAll(dir) }()
			app.gh, app.setupError = source.NewGH(r, limits, dir)
		}
	}
	if o.Command == "prs" || o.Command == "current" {
		if o.Repository != "" {
			if o.Command == "prs" {
				prs, err := app.listPullRequests(ctx, o.Repository)
				if err != nil {
					fmt.Fprintln(os.Stderr, tui.Escape(err.Error()))
					return 1
				}
				listPullRequests(os.Stdout, o.Repository, prs)
				return 0
			}
		}
		if o.Plain || os.Getenv("TERM") == "dumb" || !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
			fmt.Fprintln(os.Stderr, "prs without owner/repo requires an interactive terminal; pass owner/repo to list directly")
			return 1
		}
		resolveInteractiveTheme(&o)
		m := app.model(ctx, o)
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

	if o.Plain || os.Getenv("TERM") == "dumb" || !term.IsTerminal(os.Stdout.Fd()) || !term.IsTerminal(os.Stdin.Fd()) {
		if o.Offline {
			fmt.Fprintln(os.Stderr, "Offline snapshot; no freshness check or network access.")
		} else {
			fmt.Fprintln(os.Stderr, "Reading GitHub PR metadata; source stays local. New comparisons may fetch missing objects.")
		}
		s, e := app.load(ctx, o, func(s string) { fmt.Fprintln(os.Stderr, tui.Escape(s)) })
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
	resolveInteractiveTheme(&o)
	m := app.model(ctx, o)
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

func resolveInteractiveTheme(o *options) {
	resolved, warning, configPath := resolveThemeForInteractiveInvocation(true, o.ThemeName, globalThemeConfigPath)
	o.Theme = resolved
	o.ThemeConfigPath = configPath
	if warning != "" {
		fmt.Fprintln(os.Stderr, tui.Escape(warning))
	}
}

func checkoutFromWorkingDirectory() (string, error) {
	workingDirectory, err := os.Getwd()
	if err != nil {
		return "", err
	}
	return checkoutFromDirectory(workingDirectory)
}

func checkoutFromDirectory(directory string) (string, error) {
	checkout, err := canonicalPath(directory)
	if err != nil {
		return "", err
	}
	gitDirectory, err := os.Lstat(filepath.Join(checkout, ".git"))
	if err != nil {
		if os.IsNotExist(err) {
			return "", errors.New("open and verify must be run from a repository root")
		}
		return "", err
	}
	if !gitDirectory.IsDir() && !gitDirectory.Mode().IsRegular() {
		return "", errors.New("open and verify must be run from a repository root")
	}
	return checkout, nil
}
func main() { os.Exit(run(os.Args[1:])) }

// evalGuides reads an immutable stored snapshot and emits deterministic guide
// checks. It deliberately has no GitHub or analysis-provider dependency.
func evalGuides(store *session.Store, id string, out io.Writer) error {
	record, err := store.Load(id)
	if err != nil {
		return err
	}
	corpus, err := guideeval.CuratedCorpus()
	if err != nil {
		return err
	}
	return json.NewEncoder(out).Encode(guideeval.Evaluate(record.Guides, record.Inventory, corpus))
}
