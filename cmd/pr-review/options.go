package main

import (
	"errors"
	"flag"
	"io"
	"os"
	"regexp"
	"strings"

	"pr-review/internal/source"
)

const usage = `pr-review open <PR-URL-or-number> --repo <checkout> [--github-repo owner/repo] [--exclude pattern] [--plain]
                                  [--send-source-to-openai] [--model MODEL]
pr-review sessions
pr-review resume <id> [--offline] [--plain] [--new] [--repo <checkout>]
pr-review delete <id>
All commands accept --store <private-directory>; defaults to OS user-data storage.
Resume checks metadata unless --offline. --new creates an unreviewed comparison; retains old session.
--send-source-to-openai sends bounded pinned patches and evidence to OpenAI for guide analysis.
--model MODEL overrides the default Responses model; requires --send-source-to-openai.
env OPENAI_API_KEY is required with --send-source-to-openai and is never persisted.
Analysis is an open-only decision: resume renders stored guides and contacts no provider.`

type options struct {
	Command, SessionID, Storage string
	Identity                    source.Identity
	Checkout                    string
	Plain, Offline, New         bool
	Excludes                    []string
	// SendSource is the upload acknowledgement and the analysis switch at once,
	// so analysis cannot be enabled without naming what leaves the machine.
	SendSource bool
	Model      string
}

func parseOptions(args []string) (options, error) {
	var o options
	if len(args) == 0 {
		return o, errors.New(usage)
	}
	o.Command = args[0]
	f := flag.NewFlagSet(o.Command, flag.ContinueOnError)
	f.SetOutput(io.Discard)
	f.StringVar(&o.Storage, "store", "", "private session storage location")
	start := 2
	var repository string
	switch o.Command {
	case "open":
		f.StringVar(&o.Checkout, "repo", "", "existing local checkout")
		f.StringVar(&repository, "github-repo", "", "explicit owner/repo for numbers")
		f.BoolVar(&o.Plain, "plain", false, "non-interactive escaped text, no pager")
		f.Func("exclude", "withhold matching path (repeatable)", func(v string) error { o.Excludes = append(o.Excludes, v); return nil })
		f.BoolVar(&o.SendSource, "send-source-to-openai", false, "send bounded pinned patches and evidence to OpenAI")
		f.StringVar(&o.Model, "model", "", "OpenAI Responses model; requires --send-source-to-openai")
	case "resume":
		f.StringVar(&o.Checkout, "repo", "", "checkout override for a new comparison")
		f.BoolVar(&o.Plain, "plain", false, "non-interactive escaped text, no pager")
		f.BoolVar(&o.Offline, "offline", false, "skip metadata check; freshness unknown")
		f.BoolVar(&o.New, "new", false, "new comparison with empty progress; retain old session")
	case "sessions":
		start = 1
	case "delete":
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
	if o.Command == "open" {
		if o.Checkout == "" {
			return o, errors.New(usage)
		}
		if o.Model != "" && !o.SendSource {
			return o, errors.New("--model selects an analysis model; add --send-source-to-openai to allow the upload it needs")
		}
		if o.SendSource && strings.TrimSpace(os.Getenv("OPENAI_API_KEY")) == "" {
			return o, errors.New("--send-source-to-openai requires OPENAI_API_KEY in the environment; it is never stored")
		}
		var err error
		o.Identity, err = source.ParseIdentity(args[1], repository)
		return o, err
	}
	if o.Command == "resume" || o.Command == "delete" {
		o.SessionID = args[1]
		if !regexp.MustCompile(`^[0-9a-f]{32}$`).MatchString(o.SessionID) {
			return o, errors.New("invalid session ID")
		}
	}
	return o, nil
}
