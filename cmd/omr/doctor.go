package main

import (
	"flag"
	"os"

	"github.com/mchenziyi/oh-my-reasonix/internal/doctor"
)

func runDoctor(args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", "", "project root or a path inside the project")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	assets, _ := loadAssetsFromInvocation()
	result, runErr := doctor.Run(*projectDir, assets)
	if *jsonOutput {
		if err := writeJSONOutput(result); err != nil {
			return err
		}
	} else {
		result.Render(os.Stdout)
	}
	return runErr
}
