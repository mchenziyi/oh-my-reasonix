package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/cacheguard"
	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
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
func runQualityBenchmark(args []string) error {
	o, err := parseQualityBenchmarkOptions(args)
	if err != nil {
		return err
	}
	flags := o.flags
	fixturesRoot, resultsPath, nativeResultsPath, omrResultsPath, outputPath := o.fixturesRoot, o.resultsPath, o.nativeResultsPath, o.omrResultsPath, o.outputPath
	replay, paired, runtimeRun, runTests := o.replay, o.paired, o.runtimeRun, o.runTests
	projectDir, binary, metricsDir, eventsPath, model, configPath := o.projectDir, o.binary, o.metricsDir, o.eventsPath, o.model, o.configPath
	maxSteps, concurrency, timeout := o.maxSteps, o.concurrency, o.timeout
	minQualifiedRate, maxCost, runIDFlag := o.minQualifiedRate, o.maxCost, o.runIDFlag
	runID := "omr-" + time.Now().Format("20060102-150405")
	if strings.TrimSpace(*runIDFlag) != "" {
		runID = strings.TrimSpace(*runIDFlag)
	}
	configFile := *configPath
	if configFile == "" {
		configFile = omrconfig.FindConfig(*projectDir)
	}
	if cfg, configErr := omrconfig.Load(configFile); configErr == nil {
		if !flagWasSet(flags, "fixtures") && cfg.Fixtures != "" {
			*fixturesRoot = projectRelativePath(*projectDir, cfg.Fixtures)
		}
		if !flagWasSet(flags, "metrics-dir") && cfg.MetricsDir != "" {
			*metricsDir = projectRelativePath(*projectDir, cfg.MetricsDir)
		}
		if !flagWasSet(flags, "model") && cfg.Model != "" {
			*model = cfg.Model
		}
		if !flagWasSet(flags, "max-steps") && cfg.MaxSteps != 0 {
			*maxSteps = cfg.MaxSteps
		}
		if !flagWasSet(flags, "concurrency") && cfg.Concurrency != 0 {
			*concurrency = cfg.Concurrency
		}
		if !flagWasSet(flags, "timeout") && cfg.TimeoutSet {
			*timeout = cfg.Timeout
		}
		if !flagWasSet(flags, "min-qualified-rate") && cfg.MinQualifiedRateSet {
			*minQualifiedRate = cfg.MinQualifiedRate
		}
		if !flagWasSet(flags, "max-cost") && cfg.MaxCostSet {
			*maxCost = cfg.MaxCost
		}
	} else if !os.IsNotExist(configErr) {
		return fmt.Errorf("load OMR config: %w", configErr)
	}
	fixtures, err := qualitybench.Discover(*fixturesRoot)
	if err != nil {
		return err
	}
	if *runtimeRun && (*replay || *resultsPath != "") {
		return errors.New("--runtime cannot be combined with --replay or --results")
	}
	if *nativeResultsPath != "" || *omrResultsPath != "" {
		if *nativeResultsPath == "" || *omrResultsPath == "" || *replay || *runtimeRun || *resultsPath != "" {
			return errors.New("quality comparison requires only --native-results and --omr-results")
		}
		native, err := loadQualityResults(*nativeResultsPath)
		if err != nil {
			return err
		}
		omr, err := loadQualityResults(*omrResultsPath)
		if err != nil {
			return err
		}
		comparison := qualitybench.CompareReports(
			qualitybench.EvaluateAll(fixtures, native, runID, qualitybench.ExecutionModeReplay),
			qualitybench.EvaluateAll(fixtures, omr, runID, qualitybench.ExecutionModeReplay),
		)
		if err := writeJSONValue(*outputPath, "quality", comparison); err != nil {
			return err
		}
		if !comparison.Passed {
			return errors.New("quality comparison failed hard gate")
		}
		if err := qualitybench.CheckCostGate(comparison.OMR, *maxCost); err != nil {
			return fmt.Errorf("quality comparison cost gate failed: %w", err)
		}
		return nil
	}
	if *runtimeRun {
		if *concurrency < 1 {
			return errors.New("--concurrency must be at least 1")
		}
		if *eventsPath != "" && *concurrency > 1 {
			return errors.New("--events requires --concurrency 1 because one event stream cannot be safely shared")
		}
		results := map[string]qualitybench.RunResult{}
		var mu sync.Mutex
		sem := make(chan struct{}, *concurrency)
		var wg sync.WaitGroup
		for _, fixture := range fixtures {
			fixture := fixture
			wg.Add(1)
			go func() {
				defer wg.Done()
				sem <- struct{}{}
				defer func() { <-sem }()
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				result, runErr := qualitybench.ExecuteRuntime(ctx, fixture, *projectDir, *binary, *metricsDir, *model, *maxSteps)
				cancel()
				if runErr == nil && *eventsPath != "" {
					if events, eventErr := qualitybench.ReadEventNames(*eventsPath); eventErr == nil {
						result.Events = events
					}
				}
				if runErr != nil {
					result.Failed = true
					if result.Error == "" {
						result.Error = runErr.Error()
					}
				}
				mu.Lock()
				results[fixture.ID] = result
				mu.Unlock()
			}()
		}
		wg.Wait()
		report := qualitybench.EvaluateAll(fixtures, results, runID, qualitybench.ExecutionModeRuntime)
		if err := writeJSONValue(*outputPath, "quality", report); err != nil {
			return err
		}
		if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
			return fmt.Errorf("quality runtime failed: %w", err)
		}
		return nil
	}
	if *paired {
		nativeResults := map[string]qualitybench.RunResult{}
		omrResults := map[string]qualitybench.RunResult{}
		for _, fixture := range fixtures {
			native, omr, err := qualitybench.ReplayPaired(fixture)
			if err != nil {
				fmt.Fprintf(os.Stderr, "warning: paired replay skipped %q: %v\n", fixture.ID, err)
				continue
			}
			nativeResults[fixture.ID] = native
			omrResults[fixture.ID] = omr
		}
		nativeReport := qualitybench.EvaluateAll(fixtures, nativeResults, runID, qualitybench.ExecutionModePaired)
		omrReport := qualitybench.EvaluateAll(fixtures, omrResults, runID, qualitybench.ExecutionModePaired)
		if nativeReport.EvaluatedCount == 0 {
			return errors.New("no fixtures contain native_replay data; use --paired only on fixtures with native_replay/omr_replay")
		}
		comparison := qualitybench.CompareReports(nativeReport, omrReport)
		if err := writeJSONValue(*outputPath, "quality", comparison); err != nil {
			return err
		}
		if !comparison.Passed {
			return fmt.Errorf("paired comparison failed: native=%d/%d omr=%d/%d",
				nativeReport.QualifiedCount, nativeReport.EvaluatedCount,
				omrReport.QualifiedCount, omrReport.EvaluatedCount)
		}
		return nil
	}
	if *replay {
		results := map[string]qualitybench.RunResult{}
		for _, fixture := range fixtures {
			var result qualitybench.RunResult
			var replayErr error
			if *runTests {
				ctx, cancel := context.WithTimeout(context.Background(), *timeout)
				result, replayErr = qualitybench.ExecuteFixture(ctx, fixture, *projectDir)
				cancel()
			} else {
				result, replayErr = qualitybench.Replay(fixture)
			}
			if replayErr != nil {
				results[fixture.ID] = qualitybench.RunResult{
					Failed: true,
					Error:  replayErr.Error(),
				}
				continue
			}
			results[fixture.ID] = result
		}
		report := qualitybench.EvaluateAll(fixtures, results, runID, qualitybench.ExecutionModeReplay)
		if err := writeJSONValue(*outputPath, "quality", report); err != nil {
			return err
		}
		if report.EvaluatedCount == 0 {
			return errors.New("no fixtures contain replay outcomes")
		}
		if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
			return fmt.Errorf("quality replay failed: %w", err)
		}
		return nil
	}
	if *resultsPath == "" {
		fmt.Printf("quality fixtures: %d\n", len(fixtures))
		for _, fixture := range fixtures {
			fmt.Printf("- %s: %s\n", fixture.ID, fixture.Task)
		}
		fmt.Println("no --results supplied; execution is intentionally separate from scoring")
		return nil
	}
	results, err := loadQualityResults(*resultsPath)
	if err != nil {
		return err
	}
	report := qualitybench.EvaluateAll(fixtures, results, runID, qualitybench.ExecutionModeReplay)
	if err := writeJSONValue(*outputPath, "quality", report); err != nil {
		return err
	}
	if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
		return fmt.Errorf("quality benchmark failed: %w", err)
	}
	return nil
}
