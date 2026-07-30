package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"

	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

func runTask(args []string) error {
	if len(args) == 0 || (args[0] != "list" && args[0] != "show") {
		return errors.New("task requires list or show")
	}
	if args[0] == "show" {
		return runTaskShow(args[1:])
	}
	return runTaskList(args[1:])
}

func runTaskList(args []string) error {
	flags := flag.NewFlagSet("task list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	sessionID := flags.String("session", "", "filter by session ID")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	result, err := runner.TaskList(context.Background(), *sessionID)
	if err != nil {
		return fmt.Errorf("task list: %w", err)
	}
	if *jsonOutput {
		return writeJSONOutput(result)
	}
	if len(result.Tasks) == 0 {
		fmt.Println("No tasks found")
		return nil
	}
	fmt.Printf("%-24s %-10s %-12s %s\n", "TASK ID", "STATUS", "KIND", "STARTED")
	for _, t := range result.Tasks {
		fmt.Printf("%-24s %-10s %-12s %s\n", t.ID, t.Status, t.Kind, t.StartedAt)
	}
	return nil
}

func runTaskShow(args []string) error {
	args = normalizeLeadingTargetArgs(args)
	flags := flag.NewFlagSet("task show", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	sessionID := flags.String("session", "", "session ID for the task")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("task show requires a task-id")
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	detail, err := runner.TaskShow(context.Background(), flags.Arg(0), *sessionID)
	if err != nil {
		return fmt.Errorf("task show: %w", err)
	}
	if *jsonOutput {
		return writeJSONOutput(detail)
	}
	fmt.Printf("Task ID:           %s\n", detail.Task.ID)
	fmt.Printf("Status:            %s\n", detail.Task.Status)
	fmt.Printf("Kind:              %s\n", detail.Task.Kind)
	fmt.Printf("Session:           %s\n", detail.Task.SessionID)
	fmt.Printf("Started:           %s\n", detail.Task.StartedAt)
	if detail.Task.FinishedAt != "" {
		fmt.Printf("Finished:          %s\n", detail.Task.FinishedAt)
	}
	fmt.Printf("Artifact complete: %t\n", detail.Task.ArtifactComplete)
	return nil
}
