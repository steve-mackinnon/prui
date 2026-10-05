package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"prui/internal/source"
	"prui/internal/theme"
)

const usage = `prui
prui --version
prui [--theme NAME] [--cache-full-source]
prui open <PR-URL-or-number> [--github-repo owner/repo] [--plain] [--theme NAME] [--cache-full-source]
prui prs [owner/repo] [--plain] [--theme NAME] [--cache-full-source]
prui inbox [--account LOGIN] [--view requested|authored|participated] [--refresh] [--offline] [--plain]
  [--repository owner/repo] [--author LOGIN] [--review all|none|required|approved|changes_requested]
  [--state open|closed|all] [--draft all|yes|no] [--requests all|personal|team] [--activity all|changed|read|unknown]
prui sessions
prui resume <id> [--offline] [--plain] [--new] [--repo <checkout>] [--theme NAME]
prui eval-guides <id>
prui delete <id>
prui verify <PR-URL-or-number> --artifacts <output-directory> [--github-repo owner/repo] [--measure-runs N] [--open-timeout DURATION]
Resume checks metadata unless --offline. --new creates an unreviewed comparison; retains old session.
Open and verify must be launched from the root of the local repository checkout.`

type options struct {
	Inbox               source.InboxOptions
	InboxRefresh        bool
	Command, SessionID  string
	ThemeName           string
	ThemeConfigPath     string
	Theme               theme.Theme
	Identity            source.Identity
	Repository          string
	Checkout            string
	Artifacts           string
	MeasureRuns         int
	OpenTimeout         time.Duration
	CacheFullSource     bool
	Plain, Offline, New bool
}

func parseOptions(args []string) (options, error) {
	var o options
	if len(args) == 0 {
		o.Command = "current"
		return o, nil
	}
	implicitCurrent := strings.HasPrefix(args[0], "-")
	if implicitCurrent {
		o.Command = "current"
	} else {
		o.Command = args[0]
	}
	f := flag.NewFlagSet(o.Command, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	if o.Command == "current" || o.Command == "open" || o.Command == "resume" || o.Command == "prs" {
		f.StringVar(&o.ThemeName, "theme", "", "interactive color theme")
	}
	if o.Command == "current" || o.Command == "open" || o.Command == "prs" || o.Command == "resume" {
		f.BoolVar(&o.CacheFullSource, "cache-full-source", false, "consent to private local full OLD/NEW source storage; 1 MiB/blob, 4 MiB total, no upload")
	}
	start := 2
	var repository string
	switch o.Command {
	case "open", "verify":
		f.StringVar(&repository, "github-repo", "", "explicit owner/repo for numbers")
		if o.Command == "open" {
			f.BoolVar(&o.Plain, "plain", false, "non-interactive escaped text, no pager")
		} else {
			f.StringVar(&o.Artifacts, "artifacts", "", "new empty artifact directory")
			f.IntVar(&o.MeasureRuns, "measure-runs", 1, "number of timing observations (1-10)")
			f.DurationVar(&o.OpenTimeout, "open-timeout", 0, "maximum time to open a pull request (default 60s)")
		}
	case "resume":
		f.StringVar(&o.Checkout, "repo", "", "checkout override for a new comparison")
		f.BoolVar(&o.Plain, "plain", false, "non-interactive escaped text, no pager")
		f.BoolVar(&o.Offline, "offline", false, "skip metadata check; freshness unknown")
		f.BoolVar(&o.New, "new", false, "new comparison with empty progress; retain old session")
	case "prs":
		f.BoolVar(&o.Plain, "plain", false, "non-interactive escaped text, no pager")
		if len(args) > 1 && (args[1] == "" || args[1][0] != '-') {
			o.Repository = args[1]
			start = 2
		} else {
			start = 1
		}
	case "inbox":
		start = 1
		f.StringVar(&o.Inbox.Account, "account", "", "select cached account; refresh must match authenticated account")
		f.StringVar(&o.Inbox.View, "view", "requested", "account inbox view")
		f.StringVar(&o.Inbox.Repository, "repository", "", "repository filter")
		f.StringVar(&o.Inbox.Author, "author", "", "author filter")
		f.StringVar(&o.Inbox.Review, "review", "all", "review state")
		f.StringVar(&o.Inbox.State, "state", "open", "PR state")
		f.StringVar(&o.Inbox.Draft, "draft", "all", "draft filter")
		f.StringVar(&o.Inbox.Requests, "requests", "all", "personal/team request noise")
		f.StringVar(&o.Inbox.Activity, "activity", "all", "local activity noise")
		f.BoolVar(&o.InboxRefresh, "refresh", false, "explicitly refresh account search")
		f.BoolVar(&o.Offline, "offline", false, "cached inbox only; no credential access")
		f.BoolVar(&o.Plain, "plain", false, "escaped text output")
	case "sessions":
		start = 1
	case "current":
		if implicitCurrent {
			start = 0
		} else {
			start = 1
		}
	case "delete", "eval-guides":
	default:
		return o, errors.New(usage)
	}
	if len(args) < start {
		return o, errors.New(usage)
	}
	if err := f.Parse(args[start:]); err != nil {
		return o, err
	}
	if f.NArg() != 0 || o.Offline && o.New || o.CacheFullSource && (o.Offline || o.Command == "resume" && !o.New) {
		return o, errors.New(usage)
	}
	if o.Command == "inbox" {
		if err := o.Inbox.Validate(); err != nil {
			return o, err
		}
		if o.Offline && o.InboxRefresh {
			return o, errors.New("offline inbox cannot refresh")
		}
	}
	if o.ThemeName != "" {
		if _, err := theme.Resolve(o.ThemeName, nil); err != nil {
			return o, fmt.Errorf("invalid --theme %q; choose one of: %s", o.ThemeName, strings.Join(theme.BuiltInNames(), ", "))
		}
	}
	if o.Command == "open" || o.Command == "verify" {
		var err error
		o.Identity, err = source.ParseIdentity(args[1], repository)
		if err != nil {
			return o, err
		}
		if o.Command == "verify" && (o.Artifacts == "" || o.MeasureRuns < 1 || o.MeasureRuns > 10 || o.OpenTimeout < 0) {
			return o, errors.New(usage)
		}
		return o, nil
	}
	if o.Command == "prs" && o.Repository != "" {
		id, err := source.ParseIdentity("1", o.Repository)
		if err != nil {
			return o, err
		}
		o.Repository = id.Repository
	}
	if o.Command == "resume" || o.Command == "delete" || o.Command == "eval-guides" {
		o.SessionID = args[1]
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(o.SessionID) {
			return o, errors.New("invalid session ID")
		}
	}
	return o, nil
}
