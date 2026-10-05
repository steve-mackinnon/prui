package source

import (
	"context"
	"errors"
	"strings"
)

// PrepareHeadComparison resolves exact committed heads, including unrelated
// histories, only in the isolated object view. It never consults checkout refs.
func (v *View) PrepareHeadComparison(ctx context.Context, old, next Metadata, gh GitHub, notify func(string)) (string, error) {
	if old.Identity != next.Identity || old.BaseRepository != next.BaseRepository || old.HeadRepository != next.HeadRepository ||
		!repositoryPattern.MatchString(old.HeadRepository) || !repositoryPattern.MatchString(next.HeadRepository) ||
		!shaPattern.MatchString(old.HeadSHA) || !shaPattern.MatchString(next.HeadSHA) {
		return "", errors.New("comparison repository identity changed")
	}
	check := func() error {
		for _, sha := range []string{old.HeadSHA, next.HeadSHA} {
			if _, err := v.Git(ctx, 100, "cat-file", "-e", sha+"^{commit}"); err != nil {
				return err
			}
		}
		return nil
	}
	if err := check(); err != nil {
		if gh == nil {
			return "", errors.New("pinned head objects unavailable")
		}
		// fetch uses the repository recorded with each head, never a current ref.
		pair := Metadata{BaseRepository: old.HeadRepository, BaseSHA: old.HeadSHA, HeadRepository: next.HeadRepository, HeadSHA: next.HeadSHA}
		if notify == nil {
			notify = func(string) {}
		}
		if err := v.fetch(ctx, pair, gh, notify); err != nil {
			return "", err
		}
		if err := check(); err != nil {
			return "", err
		}
	}
	if old.HeadSHA == next.HeadSHA {
		return "same head", nil
	}
	remaining, err := v.Git(ctx, 100, "rev-list", "--max-count=1", old.HeadSHA, "^"+next.HeadSHA, "--")
	if err != nil {
		return "", err
	}
	if strings.TrimSpace(string(remaining)) == "" {
		return "forward push", nil
	}
	return "rewritten history (rebase or force push)", nil
}
