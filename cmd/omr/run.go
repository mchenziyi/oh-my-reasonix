package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

func runRun(args []string) error {
	flags := flag.NewFlagSet("run", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	eventsJSONL := flags.String("events-jsonl", "", "path to write structured events JSONL")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if flags.NArg() == 0 {
		return errors.New("run requires a task prompt")
	}
	prompt := flags.Arg(0)

	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()

	if *eventsJSONL != "" {
		result := runner.RunWithEvents(ctx, prompt, *eventsJSONL)
		// Always parse events (file is saved even on non-zero exit).
		stream, parseErr := reasonix.ParseEventStream(*eventsJSONL)
		if parseErr != nil {
			return fmt.Errorf("parse events: %w", parseErr)
		}
		if len(stream.Errors) > 0 {
			return fmt.Errorf("event stream validation failed: %s", strings.Join(stream.Errors, "; "))
		}
		if result.Err != nil {
			return fmt.Errorf("run failed (exit %d): %w", result.ExitCode, result.Err)
		}
		if *jsonOutput {
			type runOutput struct {
				Result reasonix.Result      `json:"result"`
				Events reasonix.EventStream `json:"events"`
			}
			return json.NewEncoder(os.Stdout).Encode(runOutput{Result: result, Events: stream})
		}
		fmt.Printf("Run completed (exit %d)\n", result.ExitCode)
		fmt.Printf("Events: %d, run_done=%t\n", len(stream.Events), stream.RunDone)
		if len(stream.Errors) > 0 {
			for _, e := range stream.Errors {
				fmt.Printf("  event error: %s\n", e)
			}
		}
		return nil
	}
	result := runner.RunTask(ctx, reasonix.TaskOptions{Prompt: prompt})
	if result.Err != nil {
		return fmt.Errorf("run task: %w", result.Err)
	}
	fmt.Print(result.Stdout)
	if result.Stderr != "" {
		fmt.Fprint(os.Stderr, result.Stderr)
	}
	return nil
}
