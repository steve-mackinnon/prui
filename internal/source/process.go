package source

import (
	"bytes"
	"context"
	"errors"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"syscall"
	"time"
)

var ErrLimit = errors.New("resource limit exceeded")
var ErrCommand = errors.New("command failed")
var ErrAuthentication = errors.New("GitHub authentication failed")

type Limits struct {
	Entries, ContentBytes, BlobBytes, DiffLines int
	Operation                                   time.Duration
	FetchBytes                                  int64
}

func Defaults() Limits { return Limits{10000, 50 << 20, 1 << 20, 100000, 60 * time.Second, 512 << 20} }

type Request struct {
	Program      string
	Args, Env    []string
	Stdin        []byte
	Dir          string
	Limit        int
	Timeout      time.Duration
	StorageDir   string
	StorageLimit int64
}
type Runner interface {
	Run(context.Context, Request) ([]byte, error)
}
type processRunner struct{}

func NewRunner() Runner { return processRunner{} }

type boundedBuffer struct {
	mu        sync.Mutex
	buffer    bytes.Buffer
	remaining int
	exceeded  bool
	cancel    context.CancelFunc
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	n := len(p)
	if len(p) > b.remaining {
		p = p[:b.remaining]
		b.exceeded = true
		b.cancel()
	}
	b.buffer.Write(p)
	b.remaining -= len(p)
	return n, nil
}

func (processRunner) Run(parent context.Context, r Request) ([]byte, error) {
	if r.Limit <= 0 || r.Timeout <= 0 {
		return nil, errors.New("invalid process limits")
	}
	deadline, stop := context.WithTimeout(parent, r.Timeout)
	defer stop()
	ctx, cancel := context.WithCancel(deadline)
	defer cancel()
	//nolint:gosec // Request values are constructed by this package for trusted Git/GitHub CLI invocations.
	cmd := exec.CommandContext(ctx, r.Program, r.Args...)
	cmd.Dir = r.Dir
	cmd.Env = r.Env
	if cmd.Env == nil {
		cmd.Env = []string{"PATH=/usr/bin:/bin", "LC_ALL=C"}
	}
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
	cmd.WaitDelay = time.Second
	out := &boundedBuffer{remaining: r.Limit, cancel: cancel}
	stderr := &boundedBuffer{remaining: 64 << 10, cancel: cancel}
	cmd.Stdout = out
	cmd.Stderr = stderr
	cmd.Stdin = bytes.NewReader(r.Stdin)
	if err := cmd.Start(); err != nil {
		if parent.Err() != nil {
			return nil, parent.Err()
		}
		return nil, ErrCommand
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	tick := time.NewTicker(10 * time.Millisecond)
	defer tick.Stop()
	storageExceeded := false
	for {
		select {
		case err := <-done:
			if r.StorageDir != "" {
				size, e := storageSize(r.StorageDir)
				if e != nil || size > r.StorageLimit {
					storageExceeded = true
				}
			}
			if storageExceeded || out.exceeded || stderr.exceeded {
				return nil, ErrLimit
			}
			if parent.Err() != nil {
				return nil, parent.Err()
			}
			if deadline.Err() != nil {
				return nil, deadline.Err()
			}
			if err != nil {
				return nil, ErrCommand
			}
			return out.buffer.Bytes(), nil
		case <-tick.C:
			if r.StorageDir != "" {
				size, err := storageSize(r.StorageDir)
				if err != nil || size > r.StorageLimit {
					storageExceeded = true
					cancel()
				}
			}
		}
	}
}

func storageSize(dir string) (int64, error) {
	var size int64
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		info, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		size += info.Size()
		return nil
	})
	return size, err
}

func trustedExecutable(name string) (string, error) {
	p, err := exec.LookPath(name)
	if err != nil {
		return "", err
	}
	return filepath.Abs(p)
}

func userEnvironment() []string {
	env := []string{"LC_ALL=C", "GH_PROMPT_DISABLED=1", "GH_PAGER=cat", "GH_HOST=github.com"}
	for _, key := range []string{"PATH", "HOME", "XDG_CONFIG_HOME", "GH_CONFIG_DIR", "GH_TOKEN", "GITHUB_TOKEN"} {
		if v, ok := os.LookupEnv(key); ok {
			env = append(env, key+"="+v)
		}
	}
	return env
}
