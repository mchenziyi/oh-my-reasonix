package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/mchenziyi/oh-my-reasonix/internal/cacheguard"
	omrconfig "github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/doctor"
	"github.com/mchenziyi/oh-my-reasonix/internal/install"
	"github.com/mchenziyi/oh-my-reasonix/internal/manifest"
	"github.com/mchenziyi/oh-my-reasonix/internal/qualitybench"
)

var version = "1.1.1"

func main() {
	if len(os.Args) < 2 {
		usage()
		os.Exit(2)
	}
	var err error
	switch os.Args[1] {
	case "init":
		err = runInstall(os.Args[2:], false)
	case "upgrade":
		err = runInstall(os.Args[2:], true)
	case "uninstall":
		err = runUninstall(os.Args[2:])
	case "doctor":
		err = runDoctor(os.Args[2:])
	case "config":
		err = runConfig(os.Args[2:])
	case "profile":
		err = runProfile(os.Args[2:])
	case "benchmark":
		err = runBenchmark(os.Args[2:])
	case "version":
		fmt.Printf("omr %s\n", version)
	default:
		usage()
		os.Exit(2)
	}
	if err != nil {
		fmt.Fprintln(os.Stderr, "omr:", err)
		os.Exit(1)
	}
}

func runInstall(args []string, upgrade bool) error {
	flags := flag.NewFlagSet(map[bool]string{true: "upgrade", false: "init"}[upgrade], flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", "", "project root or a path inside the project")
	dryRun := flags.Bool("dry-run", false, "show the plan without writing files")
	compose := flags.Bool("compose-prompt", false, "explicitly compose an existing user Prompt")
	allowPersist := flags.Bool("allow-persist-user-prompt", false, "confirm that a non-empty User Prompt may be persisted")
	acceptBase := flags.Bool("accept-reasonix-base-update", false, "accept a changed Reasonix base Prompt during upgrade")
	if err := flags.Parse(args); err != nil {
		return err
	}
	assets, err := loadAssetsFromInvocation()
	if err != nil {
		return err
	}
	report, runErr := install.Init(install.Options{
		ProjectDir:               *projectDir,
		DryRun:                   *dryRun,
		ComposePrompt:            *compose,
		AllowPersistUserPrompt:   *allowPersist,
		AcceptReasonixBaseUpdate: *acceptBase,
		Upgrade:                  upgrade,
		Assets:                   assets,
	})
	report.Render(os.Stdout)
	return runErr
}

func runUninstall(args []string) error {
	flags := flag.NewFlagSet("uninstall", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", "", "project root or a path inside the project")
	dryRun := flags.Bool("dry-run", false, "show the plan without writing files")
	if err := flags.Parse(args); err != nil {
		return err
	}
	report, runErr := install.Uninstall(install.Options{ProjectDir: *projectDir, DryRun: *dryRun})
	report.Render(os.Stdout)
	return runErr
}

func runDoctor(args []string) error {
	flags := flag.NewFlagSet("doctor", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", "", "project root or a path inside the project")
	if err := flags.Parse(args); err != nil {
		return err
	}
	assets, _ := loadAssetsFromInvocation()
	result, runErr := doctor.Run(*projectDir, assets)
	result.Render(os.Stdout)
	return runErr
}

func runConfig(args []string) error {
	if len(args) == 0 || args[0] != "validate" {
		return errors.New("config requires validate")
	}
	flags := flag.NewFlagSet("config validate", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	configPath := flags.String("config", "", "OMR config TOML path")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	path := *configPath
	if path == "" {
		path = filepath.Join(*projectDir, ".reasonix", "omr", "config.toml")
	}
	if _, err := omrconfig.Load(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("OMR config not found: %s", path)
		}
		return err
	}
	fmt.Printf("OMR config valid: %s\n", path)
	return nil
}

func runProfile(args []string) error {
	if len(args) == 0 || args[0] != "list" {
		return errors.New("profile requires list")
	}
	flags := flag.NewFlagSet("profile list", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project directory")
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	root, err := install.ProjectRoot(*projectDir)
	if err != nil {
		return err
	}
	m, err := manifest.Load(install.ManifestPathForDoctor(root))
	if err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("OMR manifest not found: %s", install.ManifestPathForDoctor(root))
		}
		return err
	}
	profiles := m.NormalizedProfiles()
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(profiles)
	}
	for _, profile := range profiles {
		fmt.Printf("%s\t%s\t%s\n", profile.ID, profile.Path, profile.ContentSHA256)
	}
	return nil
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
		if err := writeJSONReport(*output, comparison); err != nil {
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

func writeJSONReport(path string, value interface{}) error {
	writer := os.Stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if path != "" {
		fmt.Printf("cache report: %s\n", path)
	}
	return nil
}

func runQualityBenchmark(args []string) error {
	flags := flag.NewFlagSet("benchmark quality", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	fixturesRoot := flags.String("fixtures", "benchmarks/fixtures", "fixture root")
	resultsPath := flags.String("results", "", "optional JSON map of fixture id to RunResult")
	nativeResultsPath := flags.String("native-results", "", "Native JSON results for quality comparison")
	omrResultsPath := flags.String("omr-results", "", "OMR JSON results for quality comparison")
	outputPath := flags.String("output", "", "optional JSON report path")
	replay := flags.Bool("replay", false, "run fixtures with deterministic replay outcomes")
	runtimeRun := flags.Bool("runtime", false, "run fixtures through the real Reasonix CLI")
	runTests := flags.Bool("run-tests", false, "run fixture hidden and regression tests")
	projectDir := flags.String("project-dir", ".", "project directory for fixture tests")
	binary := flags.String("binary", "reasonix", "Reasonix executable for --runtime")
	metricsDir := flags.String("metrics-dir", "", "metrics output directory for --runtime")
	eventsPath := flags.String("events", "", "optional JSONL structured event log for --runtime")
	model := flags.String("model", "", "optional Reasonix model for --runtime")
	maxSteps := flags.Int("max-steps", 0, "optional Reasonix step limit for --runtime")
	timeout := flags.Duration("timeout", 2*time.Minute, "per benchmark execution timeout")
	minQualifiedRate := flags.Float64("min-qualified-rate", 1, "fail when qualified rate is below this value (0..1)")
	configPath := flags.String("config", "", "optional OMR config TOML (default: <project>/.reasonix/omr/config.toml)")
	if err := flags.Parse(args); err != nil {
		return err
	}
	configFile := *configPath
	if configFile == "" {
		configFile = filepath.Join(*projectDir, ".reasonix", "omr", "config.toml")
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
		if !flagWasSet(flags, "timeout") && cfg.TimeoutSet {
			*timeout = cfg.Timeout
		}
		if !flagWasSet(flags, "min-qualified-rate") && cfg.MinQualifiedRateSet {
			*minQualifiedRate = cfg.MinQualifiedRate
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
		comparison := qualitybench.CompareReports(qualitybench.EvaluateAll(fixtures, native), qualitybench.EvaluateAll(fixtures, omr))
		if err := writeJSONValue(*outputPath, comparison); err != nil {
			return err
		}
		if !comparison.Passed {
			return errors.New("quality comparison failed hard gate")
		}
		return nil
	}
	if *runtimeRun {
		results := map[string]qualitybench.RunResult{}
		for _, fixture := range fixtures {
			ctx, cancel := context.WithTimeout(context.Background(), *timeout)
			result, runErr := qualitybench.ExecuteRuntime(ctx, fixture, *projectDir, *binary, *metricsDir, *model, *maxSteps)
			cancel()
			if runErr != nil {
				// Keep evaluating the remaining fixtures so one runtime failure
				// produces a complete report instead of hiding later failures.
				results[fixture.ID] = result
				continue
			}
			if *eventsPath != "" {
				events, eventErr := qualitybench.ReadEventNames(*eventsPath)
				if eventErr != nil {
					return eventErr
				}
				result.Events = events
			}
			results[fixture.ID] = result
		}
		report := qualitybench.EvaluateAll(fixtures, results)
		if err := writeJSONValue(*outputPath, report); err != nil {
			return err
		}
		if err := qualitybench.CheckGate(report, *minQualifiedRate); err != nil {
			return fmt.Errorf("quality runtime failed: %w", err)
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
				continue
			}
			results[fixture.ID] = result
		}
		report := qualitybench.EvaluateAll(fixtures, results)
		if err := writeJSONValue(*outputPath, report); err != nil {
			return err
		}
		if report.EvaluatedCount == 0 {
			return errors.New("no fixtures contain replay outcomes")
		}
		if err := qualitybench.CheckGate(report, *minQualifiedRate); err != nil {
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
	report := qualitybench.EvaluateAll(fixtures, results)
	if err := writeJSONValue(*outputPath, report); err != nil {
		return err
	}
	if err := qualitybench.CheckGate(report, *minQualifiedRate); err != nil {
		return fmt.Errorf("quality benchmark failed: %w", err)
	}
	return nil
}

func projectRelativePath(projectDir, path string) string {
	if filepath.IsAbs(path) {
		return path
	}
	return filepath.Join(projectDir, path)
}

func flagWasSet(flags *flag.FlagSet, name string) bool {
	set := false
	flags.Visit(func(f *flag.Flag) {
		if f.Name == name {
			set = true
		}
	})
	return set
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

func writeJSONValue(path string, value interface{}) error {
	writer := os.Stdout
	var file *os.File
	if path != "" {
		var err error
		file, err = os.Create(path)
		if err != nil {
			return err
		}
		defer file.Close()
		writer = file
	}
	encoder := json.NewEncoder(writer)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(value); err != nil {
		return err
	}
	if path != "" {
		fmt.Printf("quality report: %s\n", path)
	}
	return nil
}

func loadAssetsFromInvocation() (install.Assets, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return install.Assets{}, err
	}
	return install.LoadAssets(cwd)
}

func usage() {
	name := filepath.Base(os.Args[0])
	fmt.Printf("%s init|upgrade|uninstall|doctor|config|profile|benchmark|version\n", name)
	fmt.Println("Use --help on a command for flags.")
}
