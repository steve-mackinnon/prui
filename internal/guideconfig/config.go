// Package guideconfig resolves the user's global, secret-free guide selection.
package guideconfig

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"unicode"
)

const (
	defaultModel         = "gpt-5.6-terra"
	defaultOpenAIBaseURL = "https://api.openai.com/v1"
	maxConfigBytes       = 64 << 10
	// OpenAIEnvVariable is the legacy environment variable name, not a key value.
	OpenAIEnvVariable = "OPENAI_API_KEY"
)

var envName = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)

// PathInputs supplies the environment values used to locate the user file.
type PathInputs struct {
	Home          string
	XDGConfigHome string
}

// ConfigPath follows XDG_CONFIG_HOME on macOS and Linux alike.
func ConfigPath(in PathInputs) (string, error) {
	root := in.XDGConfigHome
	if !filepath.IsAbs(root) {
		if in.Home == "" {
			return "", errors.New("guide configuration home directory is unavailable")
		}
		root = filepath.Join(in.Home, ".config")
	}
	return filepath.Join(root, "pr-review", "config.json"), nil
}

// Selection is the validated identity of one model invocation. It contains no
// credential value. BaseURL is the provider API prefix; Destination is its
// origin for the consent screen. LegacyDefault permits a pre-config cache
// entry to be reused only under the unchanged default selection.
type Selection struct {
	Provider      string
	Model         string
	BaseURL       string
	APIKeyEnv     string
	Destination   string
	LegacyDefault bool
}

// Fingerprint binds cache reuse to the model, endpoint, and guide contract.
// It excludes credential names and values, which do not determine guide output.
func (s Selection) Fingerprint(promptVersion, schemaName string) string {
	parts, _ := json.Marshal([]string{s.Provider, s.Model, s.BaseURL, promptVersion, schemaName})
	sum := sha256.Sum256(parts)
	return hex.EncodeToString(sum[:])
}

// InvalidConfigError has a fixed public message so file contents and endpoints
// cannot leak into the consent screen or saved unavailable reason.
type InvalidConfigError struct{}

func (*InvalidConfigError) Error() string { return "invalid guide configuration" }

// Load reads one global file. An absent file preserves the old OpenAI selection
// and treats OPENAI_BASE_URL as an origin; the caller passes that value in.
// Credential values are never read here.
func Load(path, legacyOpenAIBaseURL string) (Selection, error) {
	f, err := os.Open(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return defaultSelection(legacyOpenAIBaseURL)
	}
	if err != nil {
		return Selection{}, errors.New("cannot read guide configuration")
	}
	defer func() { _ = f.Close() }()
	contents, err := io.ReadAll(io.LimitReader(f, maxConfigBytes+1))
	if err != nil {
		return Selection{}, errors.New("cannot read guide configuration")
	}
	if len(contents) > maxConfigBytes {
		return Selection{}, &InvalidConfigError{}
	}
	s, err := parse(contents)
	if err != nil {
		return Selection{}, &InvalidConfigError{}
	}
	return s, nil
}

func defaultSelection(legacyBase string) (Selection, error) {
	base := defaultOpenAIBaseURL
	legacy := strings.TrimSpace(legacyBase)
	if legacy != "" {
		var err error
		base, _, err = normalizeURL(legacy)
		if err != nil {
			return Selection{}, &InvalidConfigError{}
		}
		base = appendV1(base)
	}
	_, destination, _ := normalizeURL(base)
	return Selection{Provider: "openai", Model: defaultModel, BaseURL: base, APIKeyEnv: OpenAIEnvVariable, Destination: destination, LegacyDefault: legacy == ""}, nil
}

func appendV1(base string) string {
	if strings.HasSuffix(base, "/v1") {
		return base
	}
	return base + "/v1"
}

func parse(contents []byte) (Selection, error) {
	if err := rejectDuplicateKeys(contents); err != nil {
		return Selection{}, err
	}
	var root map[string]json.RawMessage
	if err := json.Unmarshal(contents, &root); err != nil || root == nil || len(root) != 1 {
		return Selection{}, errors.New("guide configuration must contain one guide object")
	}
	raw, ok := root["guide"]
	if !ok {
		return Selection{}, errors.New("guide section is required")
	}
	var guide map[string]json.RawMessage
	if err := json.Unmarshal(raw, &guide); err != nil || guide == nil {
		return Selection{}, errors.New("guide section must be an object")
	}
	for key := range guide {
		switch key {
		case "provider", "model", "base_url", "api_key_env":
		default:
			return Selection{}, errors.New("unknown guide setting")
		}
	}
	field := func(name string) (string, bool, error) {
		raw, ok := guide[name]
		if !ok {
			return "", false, nil
		}
		var value string
		if err := json.Unmarshal(raw, &value); err != nil {
			return "", true, errors.New("guide setting must be a string")
		}
		return value, true, nil
	}
	provider, _, err := field("provider")
	if err != nil {
		return Selection{}, err
	}
	model, _, err := field("model")
	if err != nil {
		return Selection{}, err
	}
	base, hasBase, err := field("base_url")
	if err != nil {
		return Selection{}, err
	}
	keyEnv, hasKeyEnv, err := field("api_key_env")
	if err != nil {
		return Selection{}, err
	}
	if model == "" || strings.TrimSpace(model) != model || strings.IndexFunc(model, func(r rune) bool { return unicode.IsControl(r) || unicode.IsSpace(r) }) >= 0 {
		return Selection{}, errors.New("invalid model")
	}
	switch provider {
	case "openai":
		if !hasBase {
			base = defaultOpenAIBaseURL
		} else if base == "" {
			return Selection{}, errors.New("empty base URL")
		}
		if !hasKeyEnv {
			keyEnv = OpenAIEnvVariable
		}
	case "anthropic":
		if hasBase {
			return Selection{}, errors.New("base URL is unsupported for provider")
		}
		base = "https://api.anthropic.com"
		if !hasKeyEnv {
			keyEnv = "ANTHROPIC_API_KEY"
		}
	case "google":
		if hasBase {
			return Selection{}, errors.New("base URL is unsupported for provider")
		}
		base = "https://generativelanguage.googleapis.com"
		if !hasKeyEnv {
			keyEnv = "GEMINI_API_KEY"
		}
	case "openai-compatible":
		if !hasBase || base == "" {
			return Selection{}, errors.New("compatible provider requires base URL")
		}
	default:
		return Selection{}, errors.New("invalid provider")
	}
	if keyEnv == "" {
		if provider != "openai-compatible" || !isLoopbackHTTP(base) {
			return Selection{}, errors.New("credential variable is required")
		}
	} else if !envName.MatchString(keyEnv) {
		return Selection{}, errors.New("invalid credential variable")
	}
	base, destination, err := normalizeURL(base)
	if err != nil {
		return Selection{}, err
	}
	if provider == "openai" && hasBase {
		base = appendV1IfOrigin(base)
	}
	if provider == "openai-compatible" && !hasKeyEnv && !isLoopbackHTTP(base) {
		return Selection{}, errors.New("compatible provider requires credential variable")
	}
	return Selection{Provider: provider, Model: model, BaseURL: base, APIKeyEnv: keyEnv, Destination: destination}, nil
}

func appendV1IfOrigin(base string) string {
	u, _ := url.Parse(base)
	if u.Path == "" {
		return base + "/v1"
	}
	return base
}

func isLoopbackHTTP(raw string) bool {
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "http" {
		return false
	}
	host := strings.ToLower(u.Hostname())
	return host == "localhost" || (net.ParseIP(host) != nil && net.ParseIP(host).IsLoopback())
}

func normalizeURL(raw string) (base, destination string, err error) {
	if raw != strings.TrimSpace(raw) || raw == "" {
		return "", "", errors.New("invalid guide destination")
	}
	u, err := url.Parse(raw)
	if err != nil || u.Opaque != "" || u.Host == "" || u.User != nil || u.RawQuery != "" || u.ForceQuery || u.Fragment != "" || strings.Contains(raw, "#") {
		return "", "", errors.New("invalid guide destination")
	}
	if u.Scheme != "https" && (u.Scheme != "http" || !isLoopbackHTTP(raw)) {
		return "", "", errors.New("guide destination must use HTTPS or explicit loopback HTTP")
	}
	if u.Hostname() == "" || strings.ContainsAny(u.Host, " \t\r\n") || strings.HasSuffix(u.Hostname(), ".") {
		return "", "", errors.New("invalid guide destination host")
	}
	if u.RawPath != "" {
		return "", "", errors.New("encoded guide destination paths are unsupported")
	}
	// Canonicalize only components that do not change the endpoint's meaning.
	u.Scheme = strings.ToLower(u.Scheme)
	u.Host = strings.ToLower(u.Host)
	u.Path = strings.TrimRight(u.Path, "/")
	base = u.String()
	destination = u.Scheme + "://" + u.Host
	return base, destination, nil
}

func rejectDuplicateKeys(contents []byte) error {
	d := json.NewDecoder(bytes.NewReader(contents))
	if err := consumeJSON(d); err != nil {
		return err
	}
	var extra any
	if err := d.Decode(&extra); !errors.Is(err, io.EOF) {
		return errors.New("extra JSON content")
	}
	return nil
}

func consumeJSON(d *json.Decoder) error {
	token, err := d.Token()
	if err != nil {
		return err
	}
	delim, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delim {
	case '{':
		seen := make(map[string]bool)
		for d.More() {
			token, err := d.Token()
			if err != nil {
				return err
			}
			key, ok := token.(string)
			if !ok || seen[key] {
				return fmt.Errorf("duplicate or invalid key")
			}
			seen[key] = true
			if err := consumeJSON(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	case '[':
		for d.More() {
			if err := consumeJSON(d); err != nil {
				return err
			}
		}
		_, err = d.Token()
		return err
	default:
		return errors.New("invalid JSON delimiter")
	}
}
