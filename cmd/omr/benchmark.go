package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/cacheguard"
	"github.com/mchenziyi/oh-my-reasonix/internal/qualitybench"
)

type qualityBenchmarkOptions struct {
	flags                                                                    *flag.FlagSet
	fixturesRoot, resultsPath, nativeResultsPath, omrResultsPath, outputPath *string
	replay, paired, runtimeRun, runTests                                     *bool
	projectDir, binary, metricsDir, eventsPath, model, configPath            *string
	maxSteps, concurrency                                                    *int
	timeout                                                                  *time.Duration
	minQualifiedRate, maxCost                                                *float64
	runIDFlag                                                                *string
}

func parseQualityBenchmarkOptions(args []string) (qualityBenchmarkOptions, error) {
	flags := flag.NewFlagSet("benchmark quality", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	o := qualityBenchmarkOptions{flags: flags}
	o.fixturesRoot = flags.String("fixtures", "benchmarks/fixtures", "fixture root")
	o.resultsPath = flags.String("results", "", "optional JSON map of fixture id to RunResult")
	o.nativeResultsPath = flags.String("native-results", "", "Native JSON results for quality comparison")
	o.omrResultsPath = flags.String("omr-results", "", "OMR JSON results for quality comparison")
	o.outputPath = flags.String("output", "", "optional JSON report path")
	o.replay = flags.Bool("replay", false, "run fixtures with deterministic replay outcomes")
	o.paired = flags.Bool("paired", false, "run native/omr paired replay comparison (requires full-flow fixtures with native_replay/omr_replay)")
	o.runtimeRun = flags.Bool("runtime", false, "run fixtures through the real Reasonix CLI")
	o.runTests = flags.Bool("run-tests", false, "run fixture hidden and regression tests")
	o.projectDir = flags.String("project-dir", ".", "project directory for fixture tests")
	o.binary = flags.String("binary", "reasonix", "Reasonix executable for --runtime")
	o.metricsDir = flags.String("metrics-dir", "", "metrics output directory for --runtime")
	o.eventsPath = flags.String("events", "", "optional JSONL structured event log for --runtime")
	o.model = flags.String("model", "", "optional Reasonix model for --runtime")
	o.maxSteps = flags.Int("max-steps", 0, "optional Reasonix step limit for --runtime")
	o.concurrency = flags.Int("concurrency", 1, "maximum concurrent --runtime fixtures")
	o.timeout = flags.Duration("timeout", 2*time.Minute, "per benchmark execution timeout")
	o.minQualifiedRate = flags.Float64("min-qualified-rate", 1, "fail when qualified rate is below this value (0..1)")
	o.maxCost = flags.Float64("max-cost", 0, "optional aggregate cost budget; 0 disables the gate")
	o.runIDFlag = flags.String("run-id", "", "stable report run ID (default: timestamp-based)")
	o.configPath = flags.String("config", "", "optional OMR config (TOML or JSONC; default: <project>/.reasonix/omr/config.jsonc or config.toml)")
	return o, flags.Parse(args)
}

func runBenchmark(args []string) error {
	if len(args) == 0 {
		return errors.New("benchmark requires cache or quality")
	}
	switch args[0] {
	case "cache":
		return runCacheBenchmark(args[1:])
	case "quality":
		return runQualityBenchmark(args[1:])
	default:
		return fmt.Errorf("unknown benchmark %q", args[0])
	}
}

func runCacheBenchmark(args []string) error {
	flags := flag.NewFlagSet("benchmark cache", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	trace := flags.String("trace", "", "JSONL request trace")
	nativeTrace := flags.String("native-trace", "", "Native JSONL request trace for comparison")
	omrTrace := flags.String("omr-trace", "", "OMR JSONL request trace for comparison")
	output := flags.String("output", "", "optional JSON report path")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *nativeTrace != "" || *omrTrace != "" {
		if *nativeTrace == "" || *omrTrace == "" {
			return errors.New("benchmark cache comparison requires both --native-trace and --omr-trace")
		}
		native, err := cacheguard.ReadJSONL(*nativeTrace)
		if err != nil {
			return err
		}
		omr, err := cacheguard.ReadJSONL(*omrTrace)
		if err != nil {
			return err
		}
		comparison := cacheguard.CompareReports(native, omr)
		if err := writeJSONValue(*output, "cache", comparison); err != nil {
			return err
		}
		if !comparison.Passed {
			return errors.New("cache comparison failed hard gates")
		}
		return nil
	}
	if *trace == "" {
		return errors.New("benchmark cache requires --trace")
	}
	report, err := cacheguard.ReadJSONL(*trace)
	if err != nil {
		return err
	}
	if *output == "" {
		return cacheguard.WriteReport(os.Stdout, report)
	}
	file, err := os.Create(*output)
	if err != nil {
		return err
	}
	defer file.Close()
	if err := cacheguard.WriteReport(file, report); err != nil {
		return err
	}
	fmt.Printf("cache report: %s\n", *output)
	if !report.Passed {
		return errors.New("cache benchmark failed hard gates")
	}
	return nil
}

func checkQualityGates(report qualitybench.Report, minimumRate, maximumCost float64) error {
	if err := qualitybench.CheckGate(report, minimumRate); err != nil {
		return err
	}
	return qualitybench.CheckCostGate(report, maximumCost)
}

func loadQualityResults(path string) (map[string]qualitybench.RunResult, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	results := map[string]qualitybench.RunResult{}
	if err := json.Unmarshal(data, &results); err != nil {
		return nil, fmt.Errorf("parse quality results: %w", err)
	}
	return results, nil
}
