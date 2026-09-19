package verify

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
)

const (
	// MaxTranscriptBytes matches the existing PTY capture cap.
	MaxTranscriptBytes = 2 * 1024 * 1024
	// MaxScreenInputBytes bounds work before a fixed-size screen is rendered.
	MaxScreenInputBytes = 128 * 1024
)

var screenName = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ArtifactWriter writes verifier evidence below one explicit artifact directory.
// It never accepts an artifact path from a pull request or terminal screen.
type ArtifactWriter struct{ dir string }

// ScreenArtifact names the two relative files written for one screen milestone.
type ScreenArtifact struct {
	Text string
	SVG  string
}

func NewArtifactWriter(dir string) (*ArtifactWriter, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, err
	}
	if !info.IsDir() {
		return nil, errors.New("artifact directory is not a directory")
	}
	resolved, err := filepath.EvalSymlinks(dir)
	if err != nil {
		return nil, err
	}
	abs, err := filepath.Abs(resolved)
	if err != nil {
		return nil, err
	}
	return &ArtifactWriter{dir: abs}, nil
}

func (w *ArtifactWriter) WriteTranscript(transcript []byte) (string, error) {
	if len(transcript) > MaxTranscriptBytes {
		return "", errArtifactLimit
	}
	const path = "terminal.txt"
	if err := w.write(path, transcript); err != nil {
		return "", err
	}
	return path, nil
}

func (w *ArtifactWriter) WriteScreen(name, screen string) (ScreenArtifact, error) {
	if !screenName.MatchString(name) {
		return ScreenArtifact{}, errUnsafeScreenName
	}
	if len(screen) > MaxScreenInputBytes {
		return ScreenArtifact{}, errArtifactLimit
	}
	text, svg := renderScreen(screen)
	artifact := ScreenArtifact{
		Text: filepath.ToSlash(filepath.Join("screens", name+".txt")),
		SVG:  filepath.ToSlash(filepath.Join("screens", name+".svg")),
	}
	if err := w.ensureScreensDir(); err != nil {
		return ScreenArtifact{}, err
	}
	if err := w.write(artifact.Text, []byte(text)); err != nil {
		return ScreenArtifact{}, err
	}
	if err := w.write(artifact.SVG, []byte(svg)); err != nil {
		return ScreenArtifact{}, err
	}
	return artifact, nil
}

func (w *ArtifactWriter) ensureScreensDir() error {
	path := filepath.Join(w.dir, "screens")
	err := os.Mkdir(path, 0700)
	if err != nil && !errors.Is(err, fs.ErrExist) {
		return err
	}
	info, err := os.Lstat(path)
	if err != nil {
		return err
	}
	if !info.IsDir() || info.Mode()&os.ModeSymlink != 0 {
		return errUnsafeArtifactPath
	}
	return nil
}

func (w *ArtifactWriter) write(relative string, data []byte) error {
	if err := validateArtifactPath(relative); err != nil {
		return err
	}
	path := filepath.Join(w.dir, filepath.FromSlash(relative))
	contained, err := filepath.Rel(w.dir, path)
	if err != nil || contained == ".." || len(contained) >= 3 && contained[:3] == ".."+string(filepath.Separator) {
		return errUnsafeArtifactPath
	}
	f, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0600)
	if err != nil {
		return err
	}
	defer func() { _ = f.Close() }()
	_, err = f.Write(data)
	return err
}
