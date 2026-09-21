package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"pr-review/internal/source"
	"pr-review/internal/theme"
)

const usage = `pr-review
pr-review [--theme NAME]
pr-review open <PR-URL-or-number> [--github-repo owner/repo] [--plain] [--theme NAME]
pr-review prs [owner/repo] [--plain] [--theme NAME]
pr-review sessions
pr-review resume <id> [--offline] [--plain] [--new] [--repo <checkout>] [--theme NAME]
pr-review eval-guides <id>
pr-review delete <id>
pr-review verify <PR-URL-or-number> --artifacts <output-directory> [--github-repo owner/repo] [--measure-runs N] [--open-timeout DURATION]
All commands accept --store <private-directory>; defaults to OS user-data storage.
Resume checks metadata unless --offline. --new creates an unreviewed comparison; retains old session.
Open and verify must be launched from the root of the local repository checkout.`

type options struct {
	Command, SessionID, Storage string
	ThemeName                   string
	ThemeConfigPath             string
	Theme                       theme.Theme
	Identity                    source.Identity
	Repository                  string
	Checkout                    string
	Artifacts                   string
	MeasureRuns                 int
	OpenTimeout                 time.Duration
	Plain, Offline, New         bool
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
	f.StringVar(&o.Storage, "store", "", "private session storage location")
	if o.Command == "current" || o.Command == "open" || o.Command == "resume" || o.Command == "prs" {
		f.StringVar(&o.ThemeName, "theme", "", "interactive color theme")
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
	if f.NArg() != 0 || o.Offline && o.New {
		return o, errors.New(usage)
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
