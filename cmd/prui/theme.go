package main

import (
	"os"
	"runtime"

	"prui/internal/theme"
)

// resolveThemeForInvocation leaves non-interactive output entirely independent
// of user preference files. The supplied path lookup keeps that boundary easy
// to test and ensures --plain never touches theme configuration.
func resolveThemeForInvocation(interactive bool, cliName string, configPath func() (string, error)) (theme.Theme, string) {
	resolved, warning, _ := resolveThemeForInteractiveInvocation(interactive, cliName, configPath)
	return resolved, warning
}

func resolveThemeForInteractiveInvocation(interactive bool, cliName string, configPath func() (string, error)) (theme.Theme, string, string) {
	terminal, _ := theme.Resolve(theme.Terminal, nil)
	if !interactive {
		return terminal, "", ""
	}
	path, err := configPath()
	if err != nil {
		return terminal, "Theme configuration ignored; using terminal theme.", ""
	}
	resolved, warning, savable := resolveThemeConfigDetails(cliName, path)
	if !savable {
		return resolved, warning, ""
	}
	return resolved, warning, path
}

// resolveThemeConfig applies CLI name over the persisted built-in while
// retaining validated token overrides. Its warning deliberately never includes
// file contents or paths.
func resolveThemeConfig(cliName, path string) (theme.Theme, string) {
	resolved, warning, _ := resolveThemeConfigDetails(cliName, path)
	return resolved, warning
}

// resolveThemeConfigDetails identifies whether it is safe to offer persistence
// for this file. A malformed or inaccessible existing file remains read-only.
func resolveThemeConfigDetails(cliName, path string) (theme.Theme, string, bool) {
	terminal, _ := theme.Resolve(theme.Terminal, nil)
	config, exists, err := theme.LoadConfig(path)
	if err != nil {
		name := cliName
		if name == "" {
			name = theme.Terminal
		}
		resolved, resolveErr := theme.Resolve(name, nil)
		if resolveErr != nil {
			return terminal, "Theme configuration ignored; using terminal theme.", false
		}
		return resolved, "Theme configuration ignored; using " + name + " theme.", false
	}
	name := theme.Terminal
	if exists {
		name = config.Theme
	}
	if cliName != "" {
		name = cliName
	}
	resolved, err := theme.Resolve(name, config.Colors)
	if err != nil {
		return terminal, "Theme configuration ignored; using terminal theme.", false
	}
	return resolved, "", true
}

func globalThemeConfigPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return theme.ConfigPath(theme.ConfigPathInputs{
		GOOS: runtime.GOOS, Home: home, XDGConfigHome: os.Getenv("XDG_CONFIG_HOME"),
	})
}
