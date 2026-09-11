package git

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
)

// Error is returned when git exits non-zero.
type Error struct {
	Args     []string
	Dir      string
	ExitCode int
	Stderr   string
}

func (e *Error) Error() string {
	msg := strings.TrimSpace(e.Stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", e.ExitCode)
	}
	return fmt.Sprintf("git %s: %s", strings.Join(e.Args, " "), msg)
}

// baseEnv returns the environment for git subprocesses: never prompt on the
// terminal, stable English output for parsing.
func baseEnv() []string {
	return append(os.Environ(),
		"GIT_TERMINAL_PROMPT=0",
		"LC_ALL=C",
		"GIT_OPTIONAL_LOCKS=0",
	)
}

// Run executes git in dir and returns stdout. A non-zero exit yields *Error.
func Run(ctx context.Context, dir string, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = baseEnv()
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return stdout.String(), &Error{Args: args, Dir: dir, ExitCode: ee.ExitCode(), Stderr: stderr.String()}
		}
		return stdout.String(), fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return stdout.String(), nil
}

// RunStream executes git in dir and calls onLine for every line of combined
// stdout/stderr as it arrives. It returns the final error, if any. Progress
// output that uses carriage returns is split on '\r' as well as '\n'.
func RunStream(ctx context.Context, dir string, onLine func(string), args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Env = baseEnv()
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw
	if err := cmd.Start(); err != nil {
		pw.Close()
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	done := make(chan struct{})
	var lastLine string
	go func() {
		defer close(done)
		sc := bufio.NewScanner(pr)
		sc.Buffer(make([]byte, 64*1024), 1024*1024)
		sc.Split(scanLinesCR)
		for sc.Scan() {
			line := sc.Text()
			if strings.TrimSpace(line) == "" {
				continue
			}
			lastLine = line
			onLine(line)
		}
	}()
	err := cmd.Wait()
	pw.Close()
	<-done
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return &Error{Args: args, Dir: dir, ExitCode: ee.ExitCode(), Stderr: lastLine}
		}
		return fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// scanLinesCR is like bufio.ScanLines but also splits on bare '\r'.
func scanLinesCR(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}
	if i := bytes.IndexAny(data, "\r\n"); i >= 0 {
		return i + 1, data[:i], nil
	}
	if atEOF {
		return len(data), data, nil
	}
	return 0, nil, nil
}
