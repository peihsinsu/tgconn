package provider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"strings"
	"syscall"
	"time"
)

// runSubprocess executes name with args in a dedicated process group,
// respecting context cancellation/timeout and returning stdout on success.
func runSubprocess(ctx context.Context, name string, args []string) (string, error) {
	cmd := exec.Command(name, args...)
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	slog.Debug("starting subprocess", "provider", name, "args", args)
	start := time.Now()

	if err := cmd.Start(); err != nil {
		slog.Error("subprocess failed to start", "provider", name, "error", err)
		return "", fmt.Errorf("%s failed to start: %w", name, err)
	}

	pid := cmd.Process.Pid
	slog.Info("subprocess started", "provider", name, "pid", pid)

	waitDone := make(chan error, 1)
	go func() { waitDone <- cmd.Wait() }()

	select {
	case <-ctx.Done():
		elapsed := time.Since(start).Round(time.Millisecond)
		isTimeout := errors.Is(ctx.Err(), context.DeadlineExceeded)
		reason := "cancelled"
		if isTimeout {
			reason = "timeout"
		}
		slog.Warn("killing subprocess",
			"provider", name, "pid", pid,
			"reason", reason, "elapsed", elapsed,
			"stdout_bytes_so_far", stdout.Len(),
			"stderr_bytes_so_far", stderr.Len(),
		)
		killGroup(cmd.Process)
		<-waitDone

		msg := name + " cancelled"
		if isTimeout {
			msg = name + " timed out"
		}
		if s := strings.TrimSpace(stderr.String()); s != "" {
			return "", fmt.Errorf("%s: %s", msg, s)
		}
		return "", errors.New(msg)

	case err := <-waitDone:
		elapsed := time.Since(start).Round(time.Millisecond)
		if err != nil {
			stderrStr := strings.TrimSpace(stderr.String())
			stdoutStr := strings.TrimSpace(stdout.String())
			slog.Error("subprocess failed",
				"provider", name, "pid", pid,
				"elapsed", elapsed, "error", err,
				"stderr", stderrStr, "stdout", stdoutStr,
			)
			// claude CLI sometimes writes errors to stdout instead of stderr
			detail := stderrStr
			if detail == "" {
				detail = stdoutStr
			}
			if detail != "" {
				return "", fmt.Errorf("%s: %s", name, detail)
			}
			return "", fmt.Errorf("%s exited with error: %w", name, err)
		}
		slog.Info("subprocess completed",
			"provider", name, "pid", pid,
			"elapsed", elapsed, "stdout_bytes", stdout.Len(),
		)
		return stdout.String(), nil
	}
}

// killGroup sends SIGTERM to the entire process group, then SIGKILL after 3s.
func killGroup(p *os.Process) {
	if p == nil {
		return
	}
	slog.Info("sending SIGTERM to process group", "pgid", p.Pid)
	if err := syscall.Kill(-p.Pid, syscall.SIGTERM); err != nil {
		slog.Warn("SIGTERM failed", "pgid", p.Pid, "error", err)
	}
	time.AfterFunc(3*time.Second, func() {
		slog.Warn("grace period elapsed, sending SIGKILL to process group", "pgid", p.Pid)
		if err := syscall.Kill(-p.Pid, syscall.SIGKILL); err != nil {
			slog.Debug("SIGKILL failed (process likely already gone)", "pgid", p.Pid, "error", err)
		}
	})
}
