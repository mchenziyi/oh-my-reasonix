// Package reasonix contains the small, public-CLI boundary used by OMR
// diagnostics. OMR deliberately does not import Reasonix's private runtime
// packages.
package reasonix

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
)

type commandFactory func(context.Context, string, ...string) *exec.Cmd

type Runner struct {
	Binary     string
	ProjectDir string
	Env        []string

	// commandFactory is only used by package tests. Keeping the production
	// boundary as exec.CommandContext makes the CLI contract explicit.
	commandFactory commandFactory
}

type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
	Err      error
}

type Check struct {
	Name      string
	Available bool
	Detail    string
}

type Probe struct {
	Version string
	Checks  []Check
}

func (r Runner) Run(ctx context.Context, args ...string) Result {
	binary := r.Binary
	if binary == "" {
		binary = "reasonix"
	}
	factory := r.commandFactory
	if factory == nil {
		factory = exec.CommandContext
	}
	cmd := factory(ctx, binary, args...)
	if r.ProjectDir != "" {
		cmd.Dir = r.ProjectDir
	}
	if len(r.Env) > 0 {
		cmd.Env = append(os.Environ(), r.Env...)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: exitCode(err),
		Err:      err,
	}
}

func (r Runner) Probe(ctx context.Context) (Probe, error) {
	probe := Probe{}
	version := r.Run(ctx, "--version")
	if version.Err != nil {
		version = r.Run(ctx, "version")
	}
	if version.Err == nil {
		probe.Version = firstLine(version.Stdout)
		probe.Checks = append(probe.Checks, Check{Name: "version", Available: true, Detail: probe.Version})
	} else {
		probe.Checks = append(probe.Checks, Check{Name: "version", Detail: "version command unavailable"})
	}

	help := r.Run(ctx, "--help")
	if help.Err != nil {
		return probe, fmt.Errorf("reasonix --help failed (exit %d): %w", help.ExitCode, help.Err)
	}
	probe.Checks = append(probe.Checks, Check{Name: "cli", Available: true, Detail: "--help succeeded"})

	subagent := r.Run(ctx, "subagent", "--help")
	if subagent.Err != nil {
		probe.Checks = append(probe.Checks, Check{Name: "subagent", Detail: "reasonix subagent --help failed"})
		return probe, nil
	}
	probe.Checks = append(probe.Checks, Check{Name: "subagent", Available: true, Detail: "subagent command available"})
	probe.Checks = append(probe.Checks,
		Check{Name: "subagent.try", Available: hasCommand(subagent.Stdout, "try"), Detail: "read-only subagent execution"},
		Check{Name: "subagent.run", Available: hasCommand(subagent.Stdout, "run"), Detail: "normal subagent execution"},
	)

	profiles := r.Run(ctx, "subagent", "list")
	if profiles.Err != nil {
		probe.Checks = append(probe.Checks, Check{Name: "profile.list", Detail: "reasonix subagent list failed"})
		return probe, nil
	}
	probe.Checks = append(probe.Checks,
		Check{Name: "profile.list", Available: true, Detail: "profile list succeeded"},
		Check{Name: "profile.review", Available: hasProfile(profiles.Stdout, "review"), Detail: "built-in review profile"},
	)
	return probe, nil
}

func exitCode(err error) int {
	if err == nil {
		return 0
	}
	var exitErr *exec.ExitError
	if ok := errors.As(err, &exitErr); ok {
		return exitErr.ExitCode()
	}
	return -1
}

func firstLine(value string) string {
	for _, line := range strings.Split(value, "\n") {
		if trimmed := strings.TrimSpace(line); trimmed != "" {
			return trimmed
		}
	}
	return "unknown"
}

func hasCommand(help, name string) bool {
	return hasWord(help, name)
}

func hasWord(value, want string) bool {
	for _, field := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9')
	}) {
		if field == strings.ToLower(want) {
			return true
		}
	}
	return false
}

func hasProfile(value, want string) bool {
	for _, field := range strings.FieldsFunc(strings.ToLower(value), func(r rune) bool {
		return (r < 'a' || r > 'z') && (r < '0' || r > '9') && r != '-' && r != '_'
	}) {
		if field == strings.ToLower(want) {
			return true
		}
	}
	return false
}
