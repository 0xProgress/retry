package main

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"os/exec"
	"os/signal"
	"regexp"
	"strconv"
	"strings"
	"sync/atomic"
	"syscall"
	"time"
)

var (
	errHelp    = errors.New("help")
	errVersion = errors.New("version")
)

const version = "1.0.1"

const (
	ExitSuccess        = 0
	ExitFailure        = 1
	ExitInvalidArgs    = 2
	ExitCommandNotFound = 3
)

type BackoffStrategy int

const (
	BackoffLinear      BackoffStrategy = iota
	BackoffExponential
	BackoffFixed
)

type Config struct {
	MaxAttempts int
	BaseDelay   time.Duration
	Backoff     BackoffStrategy
	RetryIf     *regexp.Regexp
	Verbose     bool
	NoColor     bool
	Command     string
	Args        []string
}

func (c Config) UseColor() bool {
	if c.NoColor {
		return false
	}
	fileInfo, err := os.Stderr.Stat()
	if err != nil {
		return false
	}
	return (fileInfo.Mode() & os.ModeCharDevice) != 0
}

func parseArgs() (*Config, error) {
	cfg := &Config{
		MaxAttempts: 5,
		BaseDelay:   time.Second,
		Backoff:     BackoffLinear,
	}

	args := os.Args[1:]
	i := 0
	sepFound := false

	for i < len(args) {
		if args[i] == "--" {
			sepFound = true
			i++
			break
		}

		switch args[i] {
		case "-t", "--times":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--times requires a value")
			}
			val, err := strconv.Atoi(args[i])
			if err != nil || val < 1 {
				return nil, fmt.Errorf("--times must be a positive integer")
			}
			cfg.MaxAttempts = val

		case "-d", "--delay":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--delay requires a value")
			}
			val, err := strconv.Atoi(args[i])
			if err != nil || val < 1 {
				return nil, fmt.Errorf("--delay must be a positive integer")
			}
			cfg.BaseDelay = time.Duration(val) * time.Second

		case "-b", "--backoff":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--backoff requires a value")
			}
			switch args[i] {
			case "linear":
				cfg.Backoff = BackoffLinear
			case "exp", "exponential":
				cfg.Backoff = BackoffExponential
			case "fixed":
				cfg.Backoff = BackoffFixed
			default:
				return nil, fmt.Errorf("--backoff must be linear, exp, or fixed")
			}

		case "--retry-if":
			i++
			if i >= len(args) {
				return nil, fmt.Errorf("--retry-if requires a pattern")
			}
			re, err := regexp.Compile(args[i])
			if err != nil {
				return nil, fmt.Errorf("invalid regex pattern: %w", err)
			}
			cfg.RetryIf = re

		case "-v", "--verbose":
			cfg.Verbose = true

		case "--version":
			return nil, errVersion

		case "--no-color":
			cfg.NoColor = true

		case "-h", "--help":
			return nil, errHelp

		default:
			if strings.HasPrefix(args[i], "-") {
				return nil, fmt.Errorf("unknown flag: %s", args[i])
			}
			cfg.Command = args[i]
			cfg.Args = args[i+1:]
			return cfg, nil
		}
		i++
	}

	if sepFound {
		if i >= len(args) {
			return nil, fmt.Errorf("no command specified after --")
		}
		cfg.Command = args[i]
		cfg.Args = args[i+1:]
	} else if cfg.Command == "" {
		return nil, fmt.Errorf("no command specified")
	}

	return cfg, nil
}

func calculateBackoff(cfg *Config, attempt int) time.Duration {
	switch cfg.Backoff {
	case BackoffLinear:
		return cfg.BaseDelay * time.Duration(attempt)
	case BackoffExponential:
		return cfg.BaseDelay * time.Duration(math.Pow(2, float64(attempt-1)))
	case BackoffFixed:
		return cfg.BaseDelay
	default:
		return cfg.BaseDelay
	}
}

func run(cfg *Config) int {
	useColor := cfg.UseColor()
	green := ""
	red := ""
	yellow := ""
	reset := ""
	if useColor {
		green = "\033[32m"
		red = "\033[31m"
		yellow = "\033[33m"
		reset = "\033[0m"
	}

	for attempt := 1; attempt <= cfg.MaxAttempts; attempt++ {
		cmd := exec.Command(cfg.Command, cfg.Args...)
		cmd.Stdin = os.Stdin
		cmd.Stdout = os.Stdout

		var stderrBuf strings.Builder
		cmd.Stderr = io.MultiWriter(os.Stderr, &stderrBuf)

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

		startTime := time.Now()
		err := cmd.Start()
		if err != nil {
			signal.Stop(sigCh)
			close(sigCh)
			fmt.Fprintf(os.Stderr, "%serror:%s %v\n", red, reset, err)
			return ExitCommandNotFound
		}

		var interrupted atomic.Bool
		go func() {
			for sig := range sigCh {
				interrupted.Store(true)
				_ = cmd.Process.Signal(sig)
			}
		}()

		err = cmd.Wait()
		elapsed := time.Since(startTime)
		signal.Stop(sigCh)
		close(sigCh)

		if err == nil {
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "%s✓%s attempt %d succeeded (took %v)\n",
					green, reset, attempt, elapsed.Round(time.Millisecond))
			}
			return ExitSuccess
		}

		exitCode := ExitFailure
		if exitErr, ok := err.(*exec.ExitError); ok {
			if status, ok := exitErr.Sys().(syscall.WaitStatus); ok {
				if status.Signaled() {
					exitCode = 128 + int(status.Signal())
				} else {
					exitCode = status.ExitStatus()
				}
			}
		}

		if interrupted.Load() {
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "%s✗%s interrupted\n", red, reset)
			}
			return exitCode
		}

		if cfg.RetryIf != nil {
			if !cfg.RetryIf.MatchString(stderrBuf.String()) {
				if cfg.Verbose {
					fmt.Fprintf(os.Stderr, "%s✗%s stderr doesn't match retry-if pattern, giving up\n",
						red, reset)
				}
				return exitCode
			}
		}

		if attempt == cfg.MaxAttempts {
			if cfg.Verbose {
				fmt.Fprintf(os.Stderr, "%s✗%s attempt %d failed (took %v), no more retries\n",
					red, reset, attempt, elapsed.Round(time.Millisecond))
			}
			return exitCode
		}

		waitDuration := calculateBackoff(cfg, attempt)
		if cfg.Verbose {
			fmt.Fprintf(os.Stderr, "%s⚠%s attempt %d failed (took %v), retrying in %v...\n",
				yellow, reset, attempt, elapsed.Round(time.Millisecond), waitDuration)
		}

		time.Sleep(waitDuration)
	}

	return ExitFailure
}

const helpText = `Usage: retry [OPTIONS] [--] COMMAND [ARGS...]

Retry COMMAND with exponential backoff until it succeeds or retries are exhausted.

Options:
  -t, --times N       Maximum number of attempts (default: 5)
  -d, --delay N       Base delay in seconds (default: 1)
  -b, --backoff TYPE  Backoff strategy: linear, exp, or fixed (default: linear)
      --retry-if RE   Only retry when stderr matches regex pattern
  -v, --verbose       Print attempt count and backoff details
      --no-color      Disable colored output
      --version       Print version
  -h, --help          Show this help message

Backoff strategies:
  linear   delay × attempt         (1s, 2s, 3s, 4s, 5s)
  exp      delay × 2^(attempt-1)   (1s, 2s, 4s, 8s, 16s)
  fixed    delay every time        (5s, 5s, 5s, 5s, 5s)

Exit codes:
  0   Command succeeded on an attempt
  1   Command failed after all retries
  2   Invalid arguments or flags
  3   Command not found

Examples:
  retry --times 3 -- ping google.com
  retry -t 5 -b exp -- curl -s https://flaky-api.com
  retry --times 10 --retry-if "deadlock|timeout" -- ./migrate-db
  retry -v -- npm publish

0x suite — zero friction tools.
https://github.com/0xProgress/retry
`

func main() {
	cfg, err := parseArgs()
	if err != nil {
		if errors.Is(err, errHelp) {
			fmt.Print(helpText)
			os.Exit(ExitSuccess)
		}
		if errors.Is(err, errVersion) {
			fmt.Println("retry version", version)
			os.Exit(ExitSuccess)
		}
		fmt.Fprintf(os.Stderr, "retry: %v\n", err)
		fmt.Fprintf(os.Stderr, "Try 'retry --help' for usage.\n")
		os.Exit(ExitInvalidArgs)
	}

	os.Exit(run(cfg))
}