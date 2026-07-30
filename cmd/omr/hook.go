package main

import (
	"context"
	"flag"
	"fmt"
	"os"

	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

func runHook(args []string) error {
	// Strip "doctor" subcommand name so flag parsing sees the flags.
	if len(args) > 0 && args[0] == "doctor" {
		args = args[1:]
	}
	flags := flag.NewFlagSet("hook doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	binary := flags.String("binary", "reasonix", "Reasonix executable")
	homeDir := flags.String("home-dir", "", "Reasonix home directory (sets REASONIX_HOME)")
	jsonOutput := flags.Bool("json", false, "output as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	runner := reasonix.Runner{Binary: *binary, ProjectDir: *projectDir}
	if *homeDir != "" {
		runner.Env = append(runner.Env, "REASONIX_HOME="+*homeDir)
	}
	ctx := context.Background()
	listResult, listErr := runner.HookList(ctx)
	if listErr != nil {
		return fmt.Errorf("hook doctor: %w", listErr)
	}
	statusResult := runner.HookStatus(ctx)

	if *jsonOutput {
		type hookDoctorOutput struct {
			List   reasonix.HookListOutput   `json:"list"`
			Status reasonix.HookStatusOutput `json:"status"`
		}
		return writeJSONOutput(hookDoctorOutput{List: listResult, Status: statusResult})
	}
	if len(listResult.Hooks) == 0 {
		fmt.Println("No hooks found")
	}
	fmt.Printf("%-20s %-16s %-10s %s\n", "EVENT", "MATCH", "SCOPE", "STATUS")
	for _, h := range listResult.Hooks {
		fmt.Printf("%-20s %-16s %-10s %s\n", h.Event, h.Match, h.Scope, h.Status)
	}
	if statusResult.Unavailable {
		fmt.Printf("STATUS: unavailable — %s\n", statusResult.Error)
	} else {
		fmt.Printf("STATUS: trusted_project=%t project_defines=%t\n",
			statusResult.TrustedProject, statusResult.ProjectDefines)
		for _, source := range statusResult.Sources {
			fmt.Printf("SOURCE: scope=%s status=%s hooks=%d\n", source.Scope, source.Status, source.HookCount)
		}
	}
	return nil
}
