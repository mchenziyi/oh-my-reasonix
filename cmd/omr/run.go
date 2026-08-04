package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/evolution"
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
	store, evolutionEnabled := evolution.Store{}, false
	if cfgPath := config.FindConfig(*projectDir); cfgPath != "" {
		if cfg, err := config.Load(cfgPath); err == nil && cfg.Evolution.Enabled && cfg.Evolution.Mode != "disabled" {
			evolutionEnabled = true
			store, _ = evolution.NewStore(*projectDir)
		}
	}

	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	ctx := context.Background()

	autoEvents := false
	if *eventsJSONL == "" && evolutionEnabled {
		_ = os.MkdirAll(filepath.Join(*projectDir, ".reasonix", "omr", "evolution"), 0700)
		tmp, err := os.CreateTemp(filepath.Join(*projectDir, ".reasonix", "omr", "evolution"), "events-*.jsonl")
		if err == nil {
			*eventsJSONL = tmp.Name()
			tmp.Close()
			autoEvents = true
		}
	}
	if *eventsJSONL != "" {
		result := runner.RunWithEvents(ctx, prompt, *eventsJSONL)
		// Always parse events (file is saved even on non-zero exit).
		stream, parseErr := reasonix.ParseEventStream(*eventsJSONL)
		if parseErr != nil {
			if autoEvents {
				_ = os.Remove(*eventsJSONL)
				fmt.Fprintf(os.Stderr, "evolution: parse events: %v\n", parseErr)
				if evolutionEnabled {
					if recordErr := evolution.RecordRun(store, prompt, result, reasonix.EventStream{}); recordErr != nil {
						fmt.Fprintf(os.Stderr, "evolution: %v\n", recordErr)
					}
					logEvolutionObservation(store, *projectDir)
				}
				if result.Err != nil {
					return fmt.Errorf("run failed (exit %d): %w", result.ExitCode, result.Err)
				}
				return nil
			}
			return fmt.Errorf("parse events: %w", parseErr)
		}
		if len(stream.Errors) > 0 {
			if autoEvents {
				if evolutionEnabled {
					if recordErr := evolution.RecordRun(store, prompt, result, stream); recordErr != nil {
						fmt.Fprintf(os.Stderr, "evolution: %v\n", recordErr)
					}
					logEvolutionObservation(store, *projectDir)
				}
				fmt.Fprintf(os.Stderr, "evolution: event validation: %s\n", strings.Join(stream.Errors, "; "))
				if result.Err != nil {
					return fmt.Errorf("run failed (exit %d): %w", result.ExitCode, result.Err)
				}
				return nil
			}
			return fmt.Errorf("event stream validation failed: %s", strings.Join(stream.Errors, "; "))
		}
		if evolutionEnabled {
			var recordErr error
			if autoEvents {
				recordErr = evolution.RecordRunWithProposer(store, prompt, result, stream, evolution.ReasonixProposer{Runner: runner})
			} else {
				recordErr = evolution.RecordRun(store, prompt, result, stream)
			}
			if err := recordErr; err != nil {
				fmt.Fprintf(os.Stderr, "evolution: %v\n", err)
			}
			logEvolutionObservation(store, *projectDir)
		}
		if autoEvents {
			_ = os.Remove(*eventsJSONL)
		}
		if result.Err != nil {
			return fmt.Errorf("run failed (exit %d): %w", result.ExitCode, result.Err)
		}
		if *jsonOutput {
			type runOutput struct {
				Result reasonix.Result      `json:"result"`
				Events reasonix.EventStream `json:"events"`
			}
			return writeJSONOutput(runOutput{Result: result, Events: stream})
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

func logEvolutionObservation(store evolution.Store, projectDir string) {
	rolled, err := evolution.ObserveApproved(store, func(id string) error {
		if err := store.RestoreOverlay(id); err != nil {
			return err
		}
		return runInstall([]string{"--project-dir", projectDir}, true)
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "evolution: observation: %v\n", err)
	} else if len(rolled) > 0 {
		fmt.Fprintf(os.Stderr, "evolution: automatically rolled back %s\n", strings.Join(rolled, ", "))
	}
}
