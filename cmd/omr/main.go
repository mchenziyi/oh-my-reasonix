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
	"sync"
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
	case "session":
		err = runSession(os.Args[2:])
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
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args); err != nil {
		return err
	}
	assets, _ := loadAssetsFromInvocation()
	result, runErr := doctor.Run(*projectDir, assets)
	if *jsonOutput {
		if err := json.NewEncoder(os.Stdout).Encode(result); err != nil {
			return err
		}
	} else {
		result.Render(os.Stdout)
	}
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
	jsonOutput := flags.Bool("json", false, "write JSON output")
	if err := flags.Parse(args[1:]); err != nil {
		return err
	}
	path := *configPath
	if path == "" {
		path = filepath.Join(*projectDir, ".reasonix", "omr", "config.toml")
	}
	cfg, err := omrconfig.Load(path)
	if err != nil {
		if *jsonOutput {
			_ = json.NewEncoder(os.Stdout).Encode(struct {
				Path  string `json:"path"`
				Valid bool   `json:"valid"`
				Error string `json:"error"`
			}{Path: path, Error: err.Error()})
		}
		if os.IsNotExist(err) {
			return fmt.Errorf("OMR config not found: %s", path)
		}
		return err
	}
	if *jsonOutput {
		return json.NewEncoder(os.Stdout).Encode(struct {
			Path       string                           `json:"path"`
			Valid      bool                             `json:"valid"`
			Agents     map[string]omrconfig.AgentConfig `json:"agents"`
			Categories map[string]string                `json:"categories"`
		}{Path: path, Valid: true, Agents: cfg.Agents, Categories: cfg.Categories})
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
		type profileJSON struct {
			ID            string `json:"id"`
			Path          string `json:"path"`
			ContentSHA256 string `json:"content_sha256"`
			Model         string `json:"model,omitempty"`
			PromptFile    string `json:"prompt_file,omitempty"`
			ReadOnly      *bool  `json:"read_only,omitempty"`
		}
		configured := map[string]omrconfig.AgentConfig{}
		configPath := filepath.Join(root, ".reasonix", "omr", "config.toml")
		if _, statErr := os.Stat(configPath); statErr == nil {
			cfg, configErr := omrconfig.Load(configPath)
			if configErr != nil {
				return configErr
			}
			configured = cfg.Agents
		}
		output := make([]profileJSON, 0, len(profiles))
		for _, profile := range profiles {
			item := profileJSON{ID: profile.ID, Path: profile.Path, ContentSHA256: profile.ContentSHA256}
			if agent, ok := configured[profile.ID]; ok {
				item.Model, item.PromptFile, item.ReadOnly = agent.Model, agent.PromptFile, agent.ReadOnly
			}
			output = append(output, item)
		}
		return json.NewEncoder(os.Stdout).Encode(output)
	}
	for _, profile := range profiles {
		fmt.Printf("%s\t%s\t%s\n", profile.ID, profile.Path, profile.ContentSHA256)
	}
	return nil
}

func runSession(args []string) error {
	if len(args) == 0 || args[0] != "resume" {
		return errors.New("session requires resume")
	}
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
	concurrency := flags.Int("concurrency", 1, "maximum concurrent --runtime fixtures")
	timeout := flags.Duration("timeout", 2*time.Minute, "per benchmark execution timeout")
	minQualifiedRate := flags.Float64("min-qualified-rate", 1, "fail when qualified rate is below this value (0..1)")
	maxCost := flags.Float64("max-cost", 0, "optional aggregate cost budget; 0 disables the gate")
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
				mu.Lock()
				results[fixture.ID] = result
				mu.Unlock()
			}()
		}
		wg.Wait()
		report := qualitybench.EvaluateAll(fixtures, results)
		if err := writeJSONValue(*outputPath, report); err != nil {
			return err
		}
		if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
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
	report := qualitybench.EvaluateAll(fixtures, results)
	if err := writeJSONValue(*outputPath, report); err != nil {
		return err
	}
	if err := checkQualityGates(report, *minQualifiedRate, *maxCost); err != nil {
		return fmt.Errorf("quality benchmark failed: %w", err)
	}
	return nil
}

func checkQualityGates(report qualitybench.Report, minimumRate, maximumCost float64) error {
	if err := qualitybench.CheckGate(report, minimumRate); err != nil {
		return err
	}
	return qualitybench.CheckCostGate(report, maximumCost)
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
	fmt.Printf("%s init|upgrade|uninstall|doctor|config|profile|session|benchmark|version\n", name)
	fmt.Println("Use --help on a command for flags.")
}
