package main

import (
	"os"
	"os/exec"
	"regexp"
	"strings"
	"testing"
	"time"
)

func TestCalculateBackoffLinear(t *testing.T) {
	cfg := &Config{BaseDelay: time.Second, Backoff: BackoffLinear}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 3 * time.Second},
		{5, 5 * time.Second},
	}

	for _, tt := range tests {
		got := calculateBackoff(cfg, tt.attempt)
		if got != tt.expected {
			t.Errorf("attempt %d: got %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestCalculateBackoffExponential(t *testing.T) {
	cfg := &Config{BaseDelay: time.Second, Backoff: BackoffExponential}

	tests := []struct {
		attempt  int
		expected time.Duration
	}{
		{1, 1 * time.Second},
		{2, 2 * time.Second},
		{3, 4 * time.Second},
		{4, 8 * time.Second},
		{5, 16 * time.Second},
	}

	for _, tt := range tests {
		got := calculateBackoff(cfg, tt.attempt)
		if got != tt.expected {
			t.Errorf("attempt %d: got %v, want %v", tt.attempt, got, tt.expected)
		}
	}
}

func TestCalculateBackoffFixed(t *testing.T) {
	cfg := &Config{BaseDelay: 5 * time.Second, Backoff: BackoffFixed}

	tests := []struct {
		attempt int
	}{
		{1}, {2}, {3}, {5}, {10},
	}

	for _, tt := range tests {
		got := calculateBackoff(cfg, tt.attempt)
		if got != 5*time.Second {
			t.Errorf("attempt %d: got %v, want %v", tt.attempt, got, 5*time.Second)
		}
	}
}

func TestParseArgsDefaults(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"retry", "--", "echo", "hello"}
	cfg, err := parseArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxAttempts != 5 {
		t.Errorf("MaxAttempts: got %d, want 5", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != time.Second {
		t.Errorf("BaseDelay: got %v, want 1s", cfg.BaseDelay)
	}
	if cfg.Backoff != BackoffLinear {
		t.Errorf("Backoff: got %v, want linear", cfg.Backoff)
	}
	if cfg.Command != "echo" {
		t.Errorf("Command: got %q, want echo", cfg.Command)
	}
}

func TestParseArgsCustom(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"retry", "-t", "3", "-d", "2", "-b", "exp", "--retry-if", "timeout", "-v", "--", "curl", "https://api.com"}
	cfg, err := parseArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.MaxAttempts != 3 {
		t.Errorf("MaxAttempts: got %d, want 3", cfg.MaxAttempts)
	}
	if cfg.BaseDelay != 2*time.Second {
		t.Errorf("BaseDelay: got %v, want 2s", cfg.BaseDelay)
	}
	if cfg.Backoff != BackoffExponential {
		t.Errorf("Backoff: got %v, want exp", cfg.Backoff)
	}
	if cfg.RetryIf == nil || cfg.RetryIf.String() != "timeout" {
		t.Errorf("RetryIf: got %v, want timeout", cfg.RetryIf)
	}
	if !cfg.Verbose {
		t.Error("Verbose: should be true")
	}
	if cfg.Command != "curl" {
		t.Errorf("Command: got %q, want curl", cfg.Command)
	}
}

func TestParseArgsNoCommand(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"retry", "--times", "3"}
	_, err := parseArgs()
	if err == nil {
		t.Fatal("expected error for missing command")
	}
}

func TestParseArgsInvalidTimes(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	tests := [][]string{
		{"retry", "-t", "0", "--", "echo"},
		{"retry", "-t", "-5", "--", "echo"},
		{"retry", "-t", "abc", "--", "echo"},
	}

	for _, args := range tests {
		os.Args = args
		_, err := parseArgs()
		if err == nil {
			t.Errorf("expected error for args: %v", args)
		}
	}
}

func TestParseArgsInvalidBackoff(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"retry", "-b", "logarithmic", "--", "echo"}
	_, err := parseArgs()
	if err == nil {
		t.Fatal("expected error for invalid backoff")
	}
}

func TestParseArgsHelp(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"retry", "--help"}
	_, err := parseArgs()
	if err == nil || err.Error() != "help" {
		t.Fatalf("expected 'help' error, got: %v", err)
	}
}

func TestParseArgsWithoutSeparator(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"retry", "echo", "hello"}
	cfg, err := parseArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Command != "echo" {
		t.Errorf("Command: got %q, want echo", cfg.Command)
	}
	if len(cfg.Args) != 1 || cfg.Args[0] != "hello" {
		t.Errorf("Args: got %v, want [hello]", cfg.Args)
	}
}

func TestParseArgsExponentialAlias(t *testing.T) {
	oldArgs := os.Args
	defer func() { os.Args = oldArgs }()

	os.Args = []string{"retry", "-b", "exponential", "--", "echo"}
	cfg, err := parseArgs()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.Backoff != BackoffExponential {
		t.Errorf("Backoff: got %v, want exponential", cfg.Backoff)
	}
}

func TestUseColor(t *testing.T) {
	cfg := &Config{NoColor: true}
	if cfg.UseColor() {
		t.Error("UseColor should be false when NoColor is set")
	}
}

func TestRetryIfMatch(t *testing.T) {
	pattern := regexp.MustCompile("timeout|deadlock")
	stderr := "Error: connection timeout\n"

	if !pattern.MatchString(stderr) {
		t.Error("pattern should match stderr containing 'timeout'")
	}

	stderrNoMatch := "Error: permission denied\n"
	if pattern.MatchString(stderrNoMatch) {
		t.Error("pattern should not match stderr without 'timeout' or 'deadlock'")
	}
}

func TestRunCommandSuccess(t *testing.T) {
	cfg := &Config{
		MaxAttempts: 3,
		BaseDelay:   10 * time.Millisecond,
		Backoff:     BackoffFixed,
		Command:     "echo",
		Args:        []string{"hello"},
		Verbose:     false,
	}

	cmd := exec.Command(cfg.Command, cfg.Args...)
	out, err := cmd.Output()
	if err != nil {
		t.Fatalf("echo command failed: %v", err)
	}
	if strings.TrimSpace(string(out)) != "hello" {
		t.Errorf("got %q, want hello", string(out))
	}
}