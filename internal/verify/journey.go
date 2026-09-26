package verify

import (
	"bytes"
	"context"
	_ "embed"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"pr-review/internal/source"
)

const (
	defaultOpenTimeout    = 60 * time.Second
	defaultJourneyTimeout = 20 * time.Second
)

var sessionIDInPlain = regexp.MustCompile(`(?m)^Session: ([0-9a-f]{32}) \|`)

//go:embed testdata/journey.py
var journeyHarness []byte

// JourneyConfig contains only paths and a PR identity supplied by the caller.
// The runner never executes code from the checkout or the PR.
type JourneyConfig struct {
	Executable  string
	PRURL       string
	Checkout    string
	StoreDir    string
	ArtifactDir string
	Runs        int
	Python      string
	OpenTimeout time.Duration
	Timeout     time.Duration
	Environment []string // optional replacement environment, intended for synthetic tests
}

// Journey is the in-memory evidence returned to the command layer. Report
// paths are relative to ArtifactDir; transcript and screen content are bounded.
type Journey struct {
	Report     Report
	Transcript []byte
	Screens    map[string]string
}

type harnessEvidence struct {
	Transcript         string            `json:"transcript_base64"`
	Screens            map[string]string `json:"screens"`
	ProcessStartNS     int64             `json:"process_start_ns"`
	FirstReviewFrameNS int64             `json:"first_review_frame_ns"`
}

func (e harnessEvidence) localTiming() LocalTiming {
	return LocalTiming{ProcessStart: time.Duration(e.ProcessStartNS), FirstReviewFrame: time.Duration(e.FirstReviewFrameNS)}
}

// RunJourney opens the requested PR through the shipped binary, then runs a
// fixed PTY journey against offline resume. It is intentionally unsuitable for
// arbitrary terminal automation: commands and keys are not caller supplied.
func RunJourney(ctx context.Context, config JourneyConfig) (Journey, error) {
	journey := Journey{Report: NewReport(), Screens: map[string]string{}}
	if config.Python == "" {
		config.Python = "python3"
	}
	if err := validateJourneyConfig(config); err != nil {
		journey.Report.Status, journey.Report.Reason = Failed, "invalid verification configuration"
		return journey, err
	}
	id, err := source.ParseIdentity(config.PRURL, "")
	if err != nil {
		journey.Report.Status, journey.Report.Reason = Failed, "invalid pull request identity"
		return journey, err
	}
	journey.Report.PR = fmt.Sprintf("%s#%d", id.Repository, id.Number)

	harness, err := writeHarness()
	if err != nil {
		journey.Report.Status, journey.Report.Reason = Failed, "could not prepare terminal verifier"
		return journey, err
	}
	defer func() { _ = os.Remove(harness) }()

	var sessionID string
	timings := make([]TimingRun, 0, config.Runs)
	for run := 1; run <= config.Runs; run++ {
		var openTiming OpenTiming
		if run == 1 {
			openedAt := time.Now()
			opened, timing, err := runOpenWithTiming(ctx, config)
			openDuration := time.Since(openedAt)
			if err != nil {
				journey.Report.Status, journey.Report.Reason = Failed, "could not open pull request"
				journey.Report.Failure = &Failure{Stage: "open", ElapsedMS: openDuration.Milliseconds()}
				journey.Report.Checks = checks(Failed, NotRun, NotRun)
				return persistJourney(journey, config.ArtifactDir, err)
			}
			sessionID = sessionIDInPlain.FindStringSubmatch(string(opened))[1]
			openTiming = timing
		}
		evidence, err := runHarness(ctx, config, harness, sessionID)
		if err != nil {
			journey.Report.Status, journey.Report.Reason = Failed, "terminal journey did not complete"
			journey.Report.Checks = checks(Passed, Failed, NotRun)
			return persistJourney(journey, config.ArtifactDir, err)
		}
		transcript, err := base64.StdEncoding.DecodeString(evidence.Transcript)
		if err != nil || len(transcript) > MaxTranscriptBytes-len(journey.Transcript) {
			journey.Report.Status, journey.Report.Reason = Failed, "terminal transcript exceeded its limit"
			journey.Report.Checks = checks(Passed, Failed, NotRun)
			if err == nil {
				err = errArtifactLimit
			}
			return persistJourney(journey, config.ArtifactDir, err)
		}
		journey.Transcript = append(journey.Transcript, transcript...)
		for name, screen := range evidence.Screens {
			if len(screen) > MaxScreenInputBytes {
				journey.Report.Status, journey.Report.Reason = Failed, "terminal screen exceeded its limit"
				journey.Report.Checks = checks(Passed, Failed, NotRun)
				return persistJourney(journey, config.ArtifactDir, errArtifactLimit)
			}
			journey.Screens[fmt.Sprintf("run-%d-%s", run, name)] = screen
		}
		// The PTY harness records the elapsed time from launching an offline
		// resume to its first screen. Opening a PR includes remote work, so it
		// remains separate under the network bucket rather than being called
		// startup.
		cache := ColdCache
		if run > 1 {
			cache = WarmCache
		}
		timing, _ := NewTimingRun(cache)
		timing.Local = evidence.localTiming()
		timing.Network.GitHubMetadata = openTiming.GitHubMetadata
		timing.Network.PinAndInventory = openTiming.PinAndInventory
		if run == 1 {
			timing.Network.Stages = map[string]time.Duration{
				"view_setup": openTiming.ViewSetup,
				"fetch":      openTiming.Fetch,
				"merge_base": openTiming.MergeBase,
				"inventory":  openTiming.Inventory,
				"evidence":   openTiming.Evidence,
			}
		}
		timings = append(timings, timing)
	}
	journey.Report.Status = Passed
	journey.Report.SessionID = sessionID
	journey.Report.Checks = checks(Passed, Passed, Passed)
	timing := AggregateTiming(timings)
	journey.Report.Timing = &timing
	return persistJourney(journey, config.ArtifactDir, nil)
}

func checks(open, navigate, mark Status) []Check {
	return []Check{{Name: "open", Status: open}, {Name: "navigate", Status: navigate}, {Name: "mark_and_resume", Status: mark}}
}

func validateJourneyConfig(config JourneyConfig) error {
	if config.Executable == "" || config.PRURL == "" || config.Checkout == "" || config.StoreDir == "" || config.ArtifactDir == "" || config.Runs < 1 || config.Runs > 10 || config.OpenTimeout < 0 {
		return errors.New("executable, PR URL, checkout, store, artifacts, and runs are required")
	}
	for _, path := range []string{config.Checkout, config.StoreDir, config.ArtifactDir} {
		if !filepath.IsAbs(path) {
			return errors.New("journey paths must be absolute")
		}
		info, err := os.Stat(path)
		if err != nil || !info.IsDir() {
			return errors.New("journey directory is unavailable")
		}
	}
	entries, err := os.ReadDir(config.ArtifactDir)
	if err != nil || len(entries) != 0 {
		return errors.New("artifact directory must be new and empty")
	}
	if _, err := exec.LookPath(config.Python); err != nil {
		return errors.New("python 3 is required for terminal verification")
	}
	if _, err := os.Stat(config.Executable); err != nil {
		return errors.New("pr-review executable is unavailable")
	}
	return nil
}

func runOpen(ctx context.Context, config JourneyConfig) ([]byte, error) {
	output, _, err := runOpenWithTiming(ctx, config)
	return output, err
}

func runOpenWithTiming(ctx context.Context, config JourneyConfig) ([]byte, OpenTiming, error) {
	timeout := config.OpenTimeout
	if timeout == 0 {
		timeout = defaultOpenTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	//nolint:gosec // RunJourney validates the local executable and receives its fixed argument layout from the CLI.
	cmd := exec.CommandContext(ctx, config.Executable, "open", config.PRURL, "--store", config.StoreDir, "--plain")
	cmd.Dir = config.Checkout
	cmd.Env = openTimingEnvironment(config.Environment)
	var stdout, stderr boundedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err := cmd.Run()
	if err != nil || stdout.exceeded || sessionIDInPlain.FindStringSubmatch(string(stdout.Bytes())) == nil {
		return nil, OpenTiming{}, errors.New("open did not return a saved session")
	}
	timing, err := parseOpenTiming(stderr.Bytes())
	if err != nil {
		return nil, OpenTiming{}, err
	}
	return stdout.Bytes(), timing, nil
}

func openTimingEnvironment(environment []string) []string {
	base := environment
	if base == nil {
		base = os.Environ()
	}
	result := make([]string, 0, len(base)+1)
	for _, entry := range base {
		if !strings.HasPrefix(entry, OpenTimingEnvironment+"=") {
			result = append(result, entry)
		}
	}
	return append(result, OpenTimingEnvironment+"=1")
}

func runHarness(ctx context.Context, config JourneyConfig, harness, sessionID string) (harnessEvidence, error) {
	timeout := config.Timeout
	if timeout == 0 {
		timeout = defaultJourneyTimeout
	}
	ctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	//nolint:gosec // RunJourney validates the Python executable; the generated harness and remaining arguments are local.
	cmd := exec.CommandContext(ctx, config.Python, harness, config.Executable, config.StoreDir, sessionID, fmt.Sprintf("%.3f", timeout.Seconds()))
	if config.Environment != nil {
		cmd.Env = config.Environment
	}
	var stdout, stderr boundedBuffer
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	if err := cmd.Run(); err != nil || stdout.exceeded {
		return harnessEvidence{}, errors.New("PTY harness failed")
	}
	var evidence harnessEvidence
	if err := json.Unmarshal(stdout.Bytes(), &evidence); err != nil {
		return harnessEvidence{}, errors.New("PTY harness returned invalid evidence")
	}
	if len(evidence.Screens) != 3 || evidence.Transcript == "" {
		return harnessEvidence{}, errors.New("PTY harness returned incomplete evidence")
	}
	return evidence, nil
}

func persistJourney(journey Journey, artifactDir string, original error) (Journey, error) {
	writer, err := NewArtifactWriter(artifactDir)
	if err != nil {
		return journey, err
	}
	if len(journey.Transcript) > 0 {
		path, writeErr := writer.WriteTranscript(journey.Transcript)
		if writeErr != nil {
			return journey, writeErr
		}
		journey.Report.Artifacts.Transcript = path
	}
	names := make([]string, 0, len(journey.Screens))
	for name := range journey.Screens {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		screen := journey.Screens[name]
		artifact, writeErr := writer.WriteScreen(name, screen)
		if writeErr != nil {
			return journey, writeErr
		}
		journey.Report.Artifacts.Screens = append(journey.Report.Artifacts.Screens, artifact.SVG)
	}
	if original != nil {
		return journey, original
	}
	return journey, nil
}

func writeHarness() (string, error) {
	f, err := os.CreateTemp("", "pr-review-journey-*.py")
	if err != nil {
		return "", err
	}
	if _, err := f.Write(journeyHarness); err != nil {
		_ = f.Close()
		_ = os.Remove(f.Name())
		return "", err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(f.Name())
		return "", err
	}
	return f.Name(), nil
}

type boundedBuffer struct {
	bytes.Buffer
	exceeded bool
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	if len(p)+b.Len() > MaxTranscriptBytes {
		remaining := MaxTranscriptBytes - b.Len()
		if remaining > 0 {
			_, _ = b.Buffer.Write(p[:remaining])
		}
		b.exceeded = true
		return len(p), nil
	}
	return b.Buffer.Write(p)
}

func (b *boundedBuffer) Bytes() []byte { return b.Buffer.Bytes() }
