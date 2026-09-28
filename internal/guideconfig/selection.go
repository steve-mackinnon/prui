package guideconfig

import (
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"strings"
	"unicode"
)

// RememberedSelection records only the last confirmed public request identity.
// It deliberately excludes endpoint configuration and credentials.
type RememberedSelection struct {
	Provider string `json:"provider"`
	Model    string `json:"model"`
}

// RememberedPath places the app-owned preference beside the guide config.
func RememberedPath(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "guide-last.json")
}

var errInvalidRemembered = errors.New("invalid remembered guide selection")

// LoadRemembered returns an empty choice when the preference has not been saved.
func LoadRemembered(path string) (RememberedSelection, error) {
	f, err := os.Open(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return RememberedSelection{}, nil
	}
	if err != nil {
		return RememberedSelection{}, errors.New("cannot read remembered guide selection")
	}
	defer func() { _ = f.Close() }()
	contents, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return RememberedSelection{}, errors.New("cannot read remembered guide selection")
	}
	if len(contents) > maxConfigBytes || rejectDuplicateKeys(contents) != nil {
		return RememberedSelection{}, errInvalidRemembered
	}
	var fields map[string]json.RawMessage
	if json.Unmarshal(contents, &fields) != nil || len(fields) != 2 {
		return RememberedSelection{}, errInvalidRemembered
	}
	if _, ok := fields["provider"]; !ok {
		return RememberedSelection{}, errInvalidRemembered
	}
	if _, ok := fields["model"]; !ok {
		return RememberedSelection{}, errInvalidRemembered
	}
	var choice RememberedSelection
	if json.Unmarshal(contents, &choice) != nil || !validRemembered(choice) {
		return RememberedSelection{}, errInvalidRemembered
	}
	return choice, nil
}

func validRemembered(choice RememberedSelection) bool {
	switch choice.Provider {
	case "openai", "anthropic", "google", "openai-compatible":
	default:
		return false
	}
	return ValidModel(choice.Model)
}

// ValidModel applies the same model-ID rule as the guide config file.
func ValidModel(model string) bool {
	return model != "" && strings.TrimSpace(model) == model &&
		strings.IndexFunc(model, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) < 0
}

// PersistRemembered replaces a valid preference with a private atomic write.
// Malformed existing preferences are left untouched for the user to repair.
func PersistRemembered(path string, choice RememberedSelection) error {
	if !validRemembered(choice) {
		return errInvalidRemembered
	}
	if _, err := LoadRemembered(path); err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return errors.New("cannot save remembered guide selection")
	}
	f, err := os.CreateTemp(dir, ".guide-last-*.tmp")
	if err != nil {
		return errors.New("cannot save remembered guide selection")
	}
	defer func() { _ = os.Remove(f.Name()) }()
	if err := f.Chmod(0600); err != nil {
		_ = f.Close()
		return errors.New("cannot save remembered guide selection")
	}
	contents, _ := json.Marshal(choice)
	_, writeErr := f.Write(append(contents, '\n'))
	if writeErr == nil {
		writeErr = f.Sync()
	}
	closeErr := f.Close()
	if writeErr != nil || closeErr != nil {
		return errors.New("cannot save remembered guide selection")
	}
	if err := os.Rename(f.Name(), path); err != nil {
		return errors.New("cannot save remembered guide selection")
	}
	return nil
}

// AvailableSelections returns secret-free provider options in stable order.
// A native provider without selected configuration leaves Model blank for the
// UI to request an explicit ID, except for the existing OpenAI default.
func AvailableSelections(config Selection, getenv func(string) string) []Selection {
	if getenv == nil {
		return nil
	}
	native := []Selection{
		{Provider: "openai", Model: defaultModel, BaseURL: defaultOpenAIBaseURL, APIKeyEnv: OpenAIEnvVariable, Destination: "https://api.openai.com", LegacyDefault: true},
		{Provider: "anthropic", BaseURL: "https://api.anthropic.com", APIKeyEnv: "ANTHROPIC_API_KEY", Destination: "https://api.anthropic.com"},
		{Provider: "google", BaseURL: "https://generativelanguage.googleapis.com", APIKeyEnv: "GEMINI_API_KEY", Destination: "https://generativelanguage.googleapis.com"},
	}
	choices := make([]Selection, 0, 4)
	for _, choice := range native {
		if config.Provider == choice.Provider {
			choice = config
		}
		if choice.APIKeyEnv != "" && getenv(choice.APIKeyEnv) != "" {
			choices = append(choices, choice)
		}
	}
	if config.Provider == "openai-compatible" && (config.APIKeyEnv != "" && getenv(config.APIKeyEnv) != "" || config.APIKeyEnv == "" && isLoopbackHTTP(config.BaseURL)) {
		choices = append(choices, config)
	}
	return choices
}
