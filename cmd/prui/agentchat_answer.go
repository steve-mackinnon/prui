package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"regexp"

	"prui/internal/agentchat"
	"prui/internal/guide"
	"prui/internal/guideconfig"
)

var agentKeyEnvName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]{0,63}$`)

// answerAgent is called only for an explicit Send. The choice is copied from
// the thread, not read from a later active tab or changed guide selection.
func (a *application) answerAgent(ctx context.Context, choice guideconfig.Selection, manifest agentchat.Manifest) (agentchat.ResolvedAnswer, error) {
	if err := a.online(ctx); err != nil {
		return agentchat.ResolvedAnswer{}, err
	}
	if a.guideSelectionError != nil {
		return agentchat.ResolvedAnswer{}, a.guideSelectionError
	}
	if !guideconfig.ValidModel(choice.Model) {
		return agentchat.ResolvedAnswer{}, errors.New("agent model selection is invalid")
	}
	if choice.APIKeyEnv != "" && !agentKeyEnvName.MatchString(choice.APIKeyEnv) {
		return agentchat.ResolvedAnswer{}, errors.New("agent credential configuration is invalid")
	}
	key := ""
	if choice.APIKeyEnv != "" {
		key = os.Getenv(choice.APIKeyEnv)
		if key == "" {
			return agentchat.ResolvedAnswer{}, fmt.Errorf("%s is required to ask the agent", choice.APIKeyEnv)
		}
	}
	opts := guide.FantasyOptions{APIKey: key, Model: choice.Model, BaseURL: choice.BaseURL, Client: a.guideClient}
	var recipient guide.AgentAnswerer
	var err error
	switch choice.Provider {
	case "openai":
		recipient, err = guide.NewFantasyOpenAI(opts)
	case "anthropic":
		recipient, err = guide.NewFantasyAnthropic(opts)
	case "google":
		recipient, err = guide.NewFantasyGoogle(opts)
	case "openai-compatible":
		recipient, err = guide.NewFantasyCompatible(opts)
	default:
		return agentchat.ResolvedAnswer{}, errors.New("agent provider selection is invalid")
	}
	if err != nil {
		return agentchat.ResolvedAnswer{}, err
	}
	return recipient.Answer(ctx, manifest)
}
