// Package e2e drives the real php-lsp-go binary over stdio, exactly as an
// editor does.
package e2e

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	"github.com/amavis442/php-lsp-go/internal/testsupport/lspclient"
)

var binary string

// TestMain builds the server once with cgo disabled, which also proves the
// project stays pure Go.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "php-lsp-go-e2e")
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	binary = filepath.Join(dir, "php-lsp-go")
	if runtime.GOOS == "windows" {
		binary += ".exe"
	}

	build := exec.Command("go", "build", "-o", binary, "../cmd/php-lsp-go")
	build.Env = append(os.Environ(), "CGO_ENABLED=0")
	if out, err := build.CombinedOutput(); err != nil {
		fmt.Fprintf(os.Stderr, "building server: %v\n%s", err, out)
		os.Exit(1)
	}

	code := m.Run()
	_ = os.RemoveAll(dir)
	os.Exit(code)
}

type process struct {
	cmd    *exec.Cmd
	client *lspclient.Client
	stderr *bytes.Buffer
	cache  string // the server's cache directory
}

func start(t *testing.T) *process {
	t.Helper()
	return startWithCache(t, t.TempDir())
}

// startWithCache starts the server with cache as the user cache directory
// (LOCALAPPDATA on Windows, XDG_CACHE_HOME on Linux), so tests never touch
// the real one and can share one between sessions.
func startWithCache(t *testing.T, cache string) *process {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	t.Cleanup(cancel)

	cmd := exec.CommandContext(ctx, binary)
	cmd.Env = append(os.Environ(), "LOCALAPPDATA="+cache, "XDG_CACHE_HOME="+cache)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		t.Fatal(err)
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Start(); err != nil {
		t.Fatalf("starting server: %v", err)
	}

	return &process{cmd: cmd, client: lspclient.New(stdout, stdin), stderr: &stderr, cache: filepath.Join(cache, "php-lsp-go")}
}

func (p *process) initialize(t *testing.T) {
	t.Helper()
	resp, err := p.client.Call("initialize", map[string]any{"processId": os.Getpid(), "rootUri": nil, "capabilities": map[string]any{}})
	if err != nil || resp.Error != nil {
		t.Fatalf("initialize: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := p.client.Notify("initialized", map[string]any{}); err != nil {
		t.Fatalf("initialized: %v", err)
	}
}

func (p *process) exitCode(t *testing.T) int {
	t.Helper()
	err := p.cmd.Wait()
	var exitErr *exec.ExitError
	switch {
	case err == nil:
		return 0
	case errors.As(err, &exitErr):
		return exitErr.ExitCode()
	default:
		t.Fatalf("waiting for server: %v", err)
		return -1
	}
}

func TestCleanShutdownExitsZeroWithoutStderrOutput(t *testing.T) {
	p := start(t)
	p.initialize(t)

	resp, err := p.client.Call("shutdown", nil)
	if err != nil || resp.Error != nil {
		t.Fatalf("shutdown: err=%v resp.Error=%+v", err, resp.Error)
	}
	if err := p.client.Notify("exit", nil); err != nil {
		t.Fatalf("exit: %v", err)
	}

	if code := p.exitCode(t); code != 0 {
		t.Errorf("exit code = %d, want 0", code)
	}
	if p.stderr.Len() != 0 {
		t.Errorf("stderr not empty:\n%s", p.stderr)
	}
}

func TestExitWithoutShutdownExitsOne(t *testing.T) {
	p := start(t)
	p.initialize(t)

	if err := p.client.Notify("exit", nil); err != nil {
		t.Fatalf("exit: %v", err)
	}

	if code := p.exitCode(t); code != 1 {
		t.Errorf("exit code = %d, want 1", code)
	}
}
