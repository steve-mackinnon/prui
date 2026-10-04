package main

import (
	"context"
	"errors"
	"prui/internal/source"
)

func (a *application) readLifecycle(ctx context.Context, id source.Identity) (source.Lifecycle, error) {
	if err := a.online(ctx); err != nil {
		return source.Lifecycle{}, err
	}
	if a.setupError != nil {
		return source.Lifecycle{}, a.setupError
	}
	r, ok := a.gh.(source.LifecycleReader)
	if !ok {
		return source.Lifecycle{}, errors.New("GitHub lifecycle unavailable")
	}
	return r.ReadLifecycle(ctx, id)
}
func (a *application) submitLifecycle(ctx context.Context, action source.LifecycleAction) (source.LifecycleOutcome, error) {
	var out source.LifecycleOutcome
	if err := a.online(ctx); err != nil {
		return out, err
	}
	if a.setupError != nil {
		return out, a.setupError
	}
	writer, ok := a.gh.(source.LifecycleWriter)
	if !ok {
		return out, errors.New("GitHub lifecycle writes unavailable")
	}
	current, err := a.readLifecycle(ctx, action.Expected.Identity)
	if err != nil {
		return out, err
	}
	out.Snapshot = current
	out.Refreshed = true
	if err = current.ValidateAction(action); err != nil {
		return out, err
	}
	// Use freshly observed permissions and policy, retaining the confirmed pins.
	action.Expected = current
	out.Attempted = true
	writeErr := writer.WriteLifecycle(ctx, action)
	// Even cancellation is an uncertain write. Reconcile with a separate bounded
	// read context; no mutation is retried and no source snapshot is changed.
	refreshCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), source.Defaults().Operation)
	defer cancel()
	canonical, refreshErr := a.readLifecycle(refreshCtx, action.Expected.Identity)
	out.Refreshed = refreshErr == nil
	if out.Refreshed {
		out.Snapshot = canonical
	}
	out.Uncertain = writeErr != nil || refreshErr != nil || !canonical.ActionObserved(action)
	if out.Refreshed && canonical.ActionObserved(action) {
		out.Uncertain = false
		return out, nil
	}
	if refreshErr != nil {
		return out, errors.New("lifecycle attempted; canonical refresh unavailable; delivery uncertain")
	}
	if writeErr != nil {
		return out, writeErr
	}
	return out, errors.New("lifecycle attempted; target state not yet observed; refresh without repeating")
}
