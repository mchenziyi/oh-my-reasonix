package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

func runSession(args []string) error {
	if len(args) == 0 {
		return errors.New("session requires list, status, show, resume, or export")
	}
	switch args[0] {
	case "list":
		return runSessionList(args[1:])
	case "status":
		return runSessionStatus(args[1:])
	case "show":
		return runSessionShow(args[1:])
	case "recovery":
		return runSessionRecovery(args[1:])
	case "export":
		return runSessionExport(args[1:])
	case "resume":
		return runSessionResume(args)
	default:
		return fmt.Errorf("unknown session subcommand %q (use: list, status, show, resume, export)", args[0])
	}
}

func runSessionList(args []string) error {
	flags := flag.NewFlagSet("session list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	result, err := runner.SessionList(context.Background())
	if err != nil {
		return fmt.Errorf("session list: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(result)
	}
	if len(result.Sessions) == 0 {
		fmt.Println("No sessions found")
		return nil
	}
	fmt.Printf("%-24s %-10s %-8s %s\n", "BRANCH ID", "STATUS", "SCOPE", "TURN")
	for _, s := range result.Sessions {
		fmt.Printf("%-24s %-10s %-8s %d\n", s.BranchID, s.Status, s.Scope, s.Turn)
	}
	return nil
}

func runSessionStatus(args []string) error {
	flags := flag.NewFlagSet("session status", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("session status requires a branch-id")
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	detail, err := runner.SessionStatus(context.Background(), flags.Arg(0))
	if err != nil {
		return fmt.Errorf("session status: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(detail)
	}
	fmt.Printf("Branch ID:  %s\n", detail.BranchID)
	fmt.Printf("Status:     %s\n", detail.Status)
	if detail.Scope != "" {
		fmt.Printf("Scope:      %s\n", detail.Scope)
	}
	fmt.Printf("Turn:       %d\n", detail.Turn)
	fmt.Printf("Lifecycle:  %s\n", detail.Lifecycle)
	fmt.Printf("Recovered:  %t\n", detail.Recovered)
	fmt.Printf("Schema:     %d\n", detail.SchemaVersion)
	return nil
}

func runSessionShow(args []string) error {
	flags := flag.NewFlagSet("session show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("session show requires a branch-id")
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	detail, err := runner.SessionShow(context.Background(), flags.Arg(0))
	if err != nil {
		return fmt.Errorf("session show: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(detail)
	}
	fmt.Printf("Branch ID:  %s\n", detail.BranchID)
	fmt.Printf("Status:     %s\n", detail.Status)
	fmt.Printf("Turn:       %d\n", detail.Turn)
	fmt.Printf("Lifecycle:  %s\n", detail.Lifecycle)
	fmt.Printf("Schema:     %d\n", detail.SchemaVersion)
	return nil
}

func runSessionRecovery(args []string) error {
	flags := flag.NewFlagSet("session recovery", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	branchID := ""
	if flags.NArg() > 0 {
		branchID = flags.Arg(0)
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	info, err := runner.SessionRecovery(context.Background(), branchID)
	if err != nil {
		return fmt.Errorf("session recovery: %w", err)
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(info)
	}
	fmt.Printf("Branch ID:    %s\n", info.BranchID)
	fmt.Printf("Status:       %s\n", info.Status)
	fmt.Printf("Tasks Total:  %d\n", info.TasksTotal)
	fmt.Printf("Tasks Failed: %d\n", info.TasksFailed)
	return nil
}

func runSessionResume(args []string) error {
	flags := flag.NewFlagSet("session resume", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	copySession := flags.Bool("copy", false, "resume a duplicated session")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	path := *binary
	if !filepath.IsAbs(path) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return fmt.Errorf("Reasonix executable not found: %w", err)
		}
		path = resolved
	}
	commandArgs := []string{"--continue"}
	if *copySession {
		commandArgs = append(commandArgs, "--copy")
	}
	cmd := exec.Command(path, commandArgs...)
	cmd.Dir = *projectDir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

func runSessionExport(args []string) error {
	flags := flag.NewFlagSet("session export", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	out := flags.String("out", "", "diagnostic zip output path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() != 1 || flags.Arg(0) == "" {
		return errors.New("session export requires a branch id or session path")
	}
	path := *binary
	if !filepath.IsAbs(path) {
		resolved, err := exec.LookPath(path)
		if err != nil {
			return fmt.Errorf("Reasonix executable not found: %w", err)
		}
		path = resolved
	}
	commandArgs := []string{"doctor", "session", flags.Arg(0), "--zip"}
	if *out != "" {
		commandArgs = append(commandArgs, "--out", *out)
	}
	cmd := exec.Command(path, commandArgs...)
	cmd.Dir = *projectDir
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}
