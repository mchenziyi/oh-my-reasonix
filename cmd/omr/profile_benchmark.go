package main

import (
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/profilebench"
)

// runProfileBenchmark implements `omr benchmark profile`:
//
//	omr benchmark profile --profile omr-explore --replay --json
//	omr benchmark profile --matrix --json
//
// Replay mode evaluates offline, deterministic fixtures; no API key or
// provider is contacted. Reports carry an explicit non-quality claim.
func runProfileBenchmark(args []string) error {
	flags := flag.NewFlagSet("benchmark profile", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	profile := flags.String("profile", "", "restrict the run to one profile (e.g. omr-explore)")
	matrix := flags.Bool("matrix", false, "output the full per-profile matrix")
	replay := flags.Bool("replay", false, "evaluate fixtures with deterministic replay outcomes")
	fixturesRoot := flags.String("fixtures", "benchmarks/profile-quality", "profile-quality fixture root")
	output := flags.String("output", "", "optional JSON report output path")
	runID := flags.String("run-id", "", "stable report run ID (default: timestamp-based)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if !*replay {
		return fmt.Errorf("benchmark profile currently supports --replay only")
	}
	if *matrix && *profile != "" {
		return fmt.Errorf("--matrix and --profile cannot be combined")
	}
	fixtures, err := profilebench.Discover(*fixturesRoot)
	if err != nil {
		return err
	}
	if len(fixtures) == 0 {
		return fmt.Errorf("no profile-quality fixtures found under %s", *fixturesRoot)
	}
	runIDValue := "omr-profile-" + time.Now().Format("20060102-150405")
	if *runID != "" {
		runIDValue = *runID
	}
	results := map[string]profilebench.ProfileReplay{}
	for _, fixture := range fixtures {
		if *profile != "" && fixture.Profile != *profile {
			continue
		}
		if fixture.Replay == nil {
			continue
		}
		results[fixture.ID] = *fixture.Replay
	}
	var opts []profilebench.Option
	if *profile != "" {
		opts = append(opts, profilebench.WithProfile(*profile))
	}
	report := profilebench.EvaluateAll(fixtures, results, runIDValue, *matrix, opts...)
	if err := writeJSONValue(*output, "profile", report); err != nil {
		return err
	}
	return nil
}
