package theme

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

const (
	configDirectory = "prui"
	configFilename  = "theme.json"
)

// ConfigPathInputs supplies the platform values needed to derive the global
// config path. Callers provide these values so tests never consult a real home
// directory and startup can report environment lookup failures separately.
type ConfigPathInputs struct {
	GOOS          string
	Home          string
	XDGConfigHome string
}

// ConfigPath returns the platform-defined global theme configuration path.
func ConfigPath(in ConfigPathInputs) (string, error) {
	switch in.GOOS {
	case "darwin":
		if in.Home == "" {
			return "", errors.New("theme configuration home directory is unavailable")
		}
		return filepath.Join(in.Home, "Library", "Application Support", configDirectory, configFilename), nil
	case "linux":
		if filepath.IsAbs(in.XDGConfigHome) {
			return filepath.Join(in.XDGConfigHome, configDirectory, configFilename), nil
		}
		if in.Home == "" {
			return "", errors.New("theme configuration home directory is unavailable")
		}
		return filepath.Join(in.Home, ".config", configDirectory, configFilename), nil
	default:
		return "", fmt.Errorf("theme configuration is unsupported on %q", in.GOOS)
	}
}

// Config is the complete, validated on-disk configuration. Colors retains only
// values that ParseColor accepts; persistence can therefore safely preserve it.
type Config struct {
	Theme  string
	Colors map[Token]string
}

// InvalidConfigError identifies configuration that is intentionally read-only.
// Its public message is fixed so callers can show a safe diagnostic without
// echoing user-controlled file content.
type InvalidConfigError struct{ cause error }

func (e *InvalidConfigError) Error() string { return "invalid theme configuration" }
func (e *InvalidConfigError) Unwrap() error { return e.cause }

func invalidConfig(err error) error {
	return &InvalidConfigError{cause: err}
}

// LoadConfig reads and validates a complete configuration file. Missing files
// are normal and return exists=false. Any present but invalid file is returned
// as an InvalidConfigError and must not be replaced by persistence.
func LoadConfig(path string) (config Config, exists bool, err error) {
	// The caller selects the global user configuration, never a reviewed path.
	contents, err := os.ReadFile(filepath.Clean(path))
	if errors.Is(err, os.ErrNotExist) {
		return Config{}, false, nil
	}
	if err != nil {
		return Config{}, false, err
	}
	config, err = decodeConfig(contents)
	if err != nil {
		return Config{}, true, invalidConfig(err)
	}
	return config, true, nil
}

// PersistResult reports the durable state of a completed selection update.
// DurabilityWarning is non-nil only when theme.json was atomically replaced but
// the final directory synchronization failed. Callers must apply the selected
// theme in that case because the on-disk selection has already changed, while
// presenting a fixed safe warning rather than this underlying OS error.
type PersistResult struct {
	DurabilityWarning error
}

// PersistSelection atomically updates only the selected built-in name. It
// validates an existing file before writing, so malformed or unknown-key
// configuration remains unchanged and read-only. A non-nil error guarantees
// that replacement did not occur; see PersistResult for post-replacement
// durability warnings.
func PersistSelection(path, name string) (PersistResult, error) {
	return persistSelection(path, name, syncConfigDirectory)
}

func persistSelection(path, name string, syncDirectory func(string) error) (PersistResult, error) {
	if _, err := Resolve(name, nil); err != nil {
		return PersistResult{}, err
	}

	config, exists, err := LoadConfig(path)
	if err != nil {
		return PersistResult{}, err
	}
	if !exists {
		config = Config{Colors: make(map[Token]string)}
	}
	config.Theme = name

	contents, err := encodeConfig(config)
	if err != nil {
		return PersistResult{}, err
	}
	durabilityWarning, err := writeConfigAtomically(path, contents, syncDirectory)
	if err != nil {
		return PersistResult{}, err
	}
	return PersistResult{DurabilityWarning: durabilityWarning}, nil
}

func decodeConfig(contents []byte) (Config, error) {
	if err := rejectDuplicateJSONKeys(contents); err != nil {
		return Config{}, err
	}
	var root map[string]json.RawMessage
	decoder := json.NewDecoder(bytes.NewReader(contents))
	if err := decoder.Decode(&root); err != nil {
		return Config{}, err
	}
	if root == nil {
		return Config{}, errors.New("configuration must be an object")
	}
	if err := requireEOF(decoder); err != nil {
		return Config{}, err
	}

	for key := range root {
		if key != "theme" && key != "colors" {
			return Config{}, fmt.Errorf("unknown configuration key %q", key)
		}
	}

	config := Config{Theme: Terminal, Colors: make(map[Token]string)}
	if raw, ok := root["theme"]; ok {
		if err := json.Unmarshal(raw, &config.Theme); err != nil || config.Theme == "" {
			return Config{}, errors.New("theme must be a built-in name")
		}
		if _, err := Resolve(config.Theme, nil); err != nil {
			return Config{}, err
		}
	}
	if raw, ok := root["colors"]; ok {
		var colors map[string]json.RawMessage
		if err := json.Unmarshal(raw, &colors); err != nil || colors == nil {
			return Config{}, errors.New("colors must be an object")
		}
		for key, value := range colors {
			token := Token(key)
			if !isToken(token) {
				return Config{}, fmt.Errorf("unknown theme token %q", key)
			}
			var syntax string
			if err := json.Unmarshal(value, &syntax); err != nil {
				return Config{}, fmt.Errorf("theme color %q must be a string", key)
			}
			if _, err := ParseColor(syntax); err != nil {
				return Config{}, fmt.Errorf("invalid color for theme token %q: %w", key, err)
			}
			config.Colors[token] = syntax
		}
	}
	return config, nil
}

func encodeConfig(config Config) ([]byte, error) {
	if _, err := Resolve(config.Theme, config.Colors); err != nil {
		return nil, err
	}
	colors := make(map[string]string, len(config.Colors))
	for token, syntax := range config.Colors {
		colors[string(token)] = syntax
	}
	encoded, err := json.MarshalIndent(struct {
		Theme  string            `json:"theme"`
		Colors map[string]string `json:"colors,omitempty"`
	}{Theme: config.Theme, Colors: colors}, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(encoded, '\n'), nil
}

func writeConfigAtomically(path string, contents []byte, syncDirectory func(string) error) (durabilityWarning error, err error) {
	directory := filepath.Dir(path)
	if err := os.MkdirAll(directory, 0700); err != nil {
		return nil, err
	}

	temporary, err := os.CreateTemp(directory, ".theme.json-*")
	if err != nil {
		return nil, err
	}
	temporaryPath := temporary.Name()
	defer func() {
		if temporary != nil {
			_ = temporary.Close()
		}
		if err != nil {
			_ = os.Remove(temporaryPath)
		}
	}()
	if err := temporary.Chmod(0600); err != nil {
		return nil, err
	}
	if _, err := temporary.Write(contents); err != nil {
		return nil, err
	}
	if err := temporary.Sync(); err != nil {
		return nil, err
	}
	if err := temporary.Close(); err != nil {
		return nil, err
	}
	temporary = nil
	if err := os.Rename(temporaryPath, path); err != nil {
		return nil, err
	}
	if err := syncDirectory(directory); err != nil {
		return err, nil
	}
	return nil, nil
}

func syncConfigDirectory(directory string) error {
	// This is the parent of the caller-selected global configuration file.
	dir, err := os.Open(filepath.Clean(directory))
	if err != nil {
		return err
	}
	return errors.Join(dir.Sync(), dir.Close())
}

func requireEOF(decoder *json.Decoder) error {
	var extra any
	err := decoder.Decode(&extra)
	if errors.Is(err, io.EOF) {
		return nil
	}
	if err == nil {
		return errors.New("configuration contains multiple JSON values")
	}
	return err
}

// rejectDuplicateJSONKeys makes hand-maintained configuration deterministic.
// encoding/json otherwise keeps the last duplicate member, which could hide a
// misspelling or turn a seemingly safe update into an unexpected replacement.
func rejectDuplicateJSONKeys(contents []byte) error {
	decoder := json.NewDecoder(bytes.NewReader(contents))
	if err := consumeJSONValue(decoder); err != nil {
		return err
	}
	return requireEOF(decoder)
}

func consumeJSONValue(decoder *json.Decoder) error {
	token, err := decoder.Token()
	if err != nil {
		return err
	}
	delimiter, ok := token.(json.Delim)
	if !ok {
		return nil
	}
	switch delimiter {
	case '{':
		keys := make(map[string]struct{})
		for decoder.More() {
			keyToken, err := decoder.Token()
			if err != nil {
				return err
			}
			key, ok := keyToken.(string)
			if !ok {
				return errors.New("configuration object key is not a string")
			}
			if _, exists := keys[key]; exists {
				return fmt.Errorf("duplicate configuration key %q", key)
			}
			keys[key] = struct{}{}
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token != json.Delim('}') {
			return errors.New("configuration object is not closed")
		}
	case '[':
		for decoder.More() {
			if err := consumeJSONValue(decoder); err != nil {
				return err
			}
		}
		token, err := decoder.Token()
		if err != nil {
			return err
		}
		if token != json.Delim(']') {
			return errors.New("configuration array is not closed")
		}
	default:
		return errors.New("unexpected JSON delimiter")
	}
	return nil
}
