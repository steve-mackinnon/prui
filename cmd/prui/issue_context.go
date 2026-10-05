package main

import (
	"context"
	"errors"
	"os"
	"prui/internal/issuecontext"
	"prui/internal/source"
	"time"
)

// refresh=false is cache-only, including in offline mode. No configuration,
// client construction or credential access occurs until an explicit refresh.
func (a *application) readIssueContext(ctx context.Context, id source.Identity, refresh bool) (source.IssueContext, error) {
	if !refresh {
		if a.store == nil {
			return source.IssueContext{}, errors.New("issue context storage unavailable")
		}
		return a.store.LoadIssueContext(ctx, id)
	}
	if err := a.online(ctx); err != nil {
		return source.IssueContext{}, err
	}
	if a.setupError != nil {
		return source.IssueContext{}, a.setupError
	}
	reader, ok := a.gh.(source.IssueContextReader)
	if !ok {
		return source.IssueContext{}, errors.New("GitHub issue context unavailable")
	}
	ctx, cancel := context.WithTimeout(ctx, 60*time.Second)
	defer cancel()
	c, err := reader.ReadIssueContext(ctx, id)
	if err != nil {
		return source.IssueContext{}, err
	}
	config, configErr := issuecontext.LoadConfig(issuecontext.ConfigPath(a.guideConfigPath))
	if configErr != nil {
		c.LinearReason = configErr.Error()
	} else {
		linearCtx, linearCancel := context.WithTimeout(ctx, 25*time.Second)
		c.Linear, c.LinearReason = (issuecontext.Linear{Config: config, Getenv: os.Getenv, Transport: a.issueContextTransport}).Read(linearCtx, id.Repository, c.Body)
		linearCancel()
		if len(c.Linear) > 0 {
			c.LinearWorkspace, c.LinearAuth = config.Workspace, config.Auth
		}
	}
	c.Body = ""
	if c.Identity != id || !source.ValidateIssueContext(c) {
		return source.IssueContext{}, errors.New("invalid issue context")
	}
	if err = ctx.Err(); err != nil {
		if errors.Is(err, context.Canceled) {
			return source.IssueContext{}, err
		}
		return c, errors.New("context refreshed but deadline expired; not saved")
	}
	if a.store == nil {
		return c, errors.New("issue context not saved: storage unavailable")
	}
	if err = a.store.SaveIssueContext(ctx, c); err != nil {
		return c, errors.New("issue context not saved: storage failure")
	}
	return c, nil
}
