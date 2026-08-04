package main

import (
	"encoding/json"
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"github.com/mchenziyi/oh-my-reasonix/internal/config"
	"github.com/mchenziyi/oh-my-reasonix/internal/evolution"
	"github.com/mchenziyi/oh-my-reasonix/internal/fileutil"
)

func runEvolve(args []string) error {
	if len(args) == 0 {
		return errors.New("evolve requires status, proposals, report, export, import, approve, reject, history, rollback, doctor, prune, or repair")
	}
	fs := flag.NewFlagSet("evolve", flag.ContinueOnError)
	fs.SetOutput(os.Stderr)
	project := fs.String("project-dir", ".", "project directory")
	jsonOut := fs.Bool("json", false, "JSON output")
	outputPath := fs.String("output", "", "experience package output path")
	inputPath := fs.String("input", "", "experience package input path")
	dryRun := fs.Bool("dry-run", false, "preview without writing")
	keepEpisodes := fs.Int("keep-episodes", 500, "keep at most this many newest episodes when pruning")
	_ = fs.Parse(args[1:])
	// Go's flag package stops at the first positional argument. Evolve commands
	// intentionally accept `approve <id> --project-dir ...`, so recover flags
	// after the identifier as well.
	for i := 1; i < len(args); i++ {
		if args[i] == "--project-dir" && i+1 < len(args) {
			*project = args[i+1]
			i++
		} else if args[i] == "--json" {
			*jsonOut = true
		} else if args[i] == "--dry-run" {
			*dryRun = true
		} else if args[i] == "--keep-episodes" && i+1 < len(args) {
			if v, err := strconv.Atoi(args[i+1]); err == nil {
				*keepEpisodes = v
			}
			i++
		}
	}
	s, err := evolution.NewStore(*project)
	if err != nil {
		return err
	}
	switch args[0] {
	case "status":
		cfgPath := config.FindConfig(*project)
		cfg, loadErr := config.Load(cfgPath)
		if loadErr != nil && !os.IsNotExist(loadErr) {
			return loadErr
		}
		eps, e1 := s.ListEpisodes()
		if e1 != nil {
			return fmt.Errorf("evolution episodes: %w", e1)
		}
		ps, e2 := s.ListPatterns()
		if e2 != nil {
			return fmt.Errorf("evolution patterns: %w", e2)
		}
		props, e3 := s.ListProposals()
		if e3 != nil {
			return fmt.Errorf("evolution proposals: %w", e3)
		}
		out := struct {
			Enabled   bool   `json:"enabled"`
			Mode      string `json:"mode"`
			Episodes  int    `json:"episodes"`
			Patterns  int    `json:"patterns"`
			Proposals int    `json:"proposals"`
			Overlay   bool   `json:"overlay"`
		}{cfg.Evolution.Enabled, cfg.Evolution.Mode, len(eps), len(ps), len(props), false}
		if _, e := s.ReadOverlay(); e == nil {
			out.Overlay = true
		}
		return emitEvolve(out, *jsonOut)
	case "proposals":
		v, e := s.ListProposals()
		if e != nil {
			return e
		}
		return emitEvolve(v, *jsonOut)
	case "report":
		v, e := evolution.BuildReport(s)
		if e != nil {
			return e
		}
		return emitEvolve(v, *jsonOut)
	case "export":
		output := *outputPath
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--output" {
				output = args[i+1]
				break
			}
		}
		if output == "" {
			return errors.New("export requires --output path")
		}
		if err := evolution.ExportPackage(s, output); err != nil {
			return err
		}
		return emitEvolve(map[string]any{"exported": true, "path": output}, *jsonOut)
	case "import":
		input := *inputPath
		for i := 1; i+1 < len(args); i++ {
			if args[i] == "--input" {
				input = args[i+1]
				break
			}
		}
		if input == "" {
			return errors.New("import requires --input path")
		}
		result, err := evolution.ImportPackageWithOptions(s, input, evolution.ImportOptions{DryRun: *dryRun})
		if err != nil {
			if *jsonOut {
				_ = emitEvolve(result, true)
			}
			return err
		}
		return emitEvolve(result, *jsonOut)
	case "approve":
		if len(args) < 2 {
			return errors.New("approve requires id")
		}
		p, e := s.LoadProposal(args[1])
		if e != nil {
			return e
		}
		if p.Status == "approved" {
			return nil
		}
		root, _ := filepath.Abs(*project)
		promptPath := filepath.Join(root, ".reasonix", "omr", "generated", "system-prompt.md")
		manifestPath := filepath.Join(root, ".reasonix", "omr", "manifest.lock.yaml")
		oldPrompt, _ := os.ReadFile(promptPath)
		oldManifest, _ := os.ReadFile(manifestPath)
		restore := func() {
			if len(oldPrompt) > 0 {
				_ = fileutil.AtomicWrite(promptPath, oldPrompt, 0600)
			}
			if len(oldManifest) > 0 {
				_ = fileutil.AtomicWrite(manifestPath, oldManifest, 0600)
			}
		}
		experiment, gate := evolution.EvaluatePromotion(p)
		if !gate.Passed {
			if saveErr := s.SaveExperiment(experiment); saveErr != nil {
				return saveErr
			}
			if *jsonOut {
				_ = emitEvolve(gate, true)
			}
			return fmt.Errorf("promotion gate failed: %s", gate.Code)
		}
		if e = s.SaveExperiment(experiment); e != nil {
			return e
		}
		p.Status = "approved"
		p.ApprovedAt = evolution.Now()
		p.UpdatedAt = evolution.Now()
		if e = s.SnapshotOverlay(p.ID); e != nil {
			return e
		}
		if e = s.WriteOverlay(p.Overlay); e != nil {
			return e
		}
		if e = runInstall([]string{"--project-dir", *project}, true); e != nil {
			_ = s.RestoreOverlay(p.ID)
			restore()
			return fmt.Errorf("apply overlay prompt: %w", e)
		}
		if e = s.SaveProposal(p); e != nil {
			return e
		}
		if *jsonOut {
			return emitEvolve(gate, true)
		}
		return nil
	case "reject":
		if len(args) < 2 {
			return errors.New("reject requires id")
		}
		p0, pe := s.LoadProposal(args[1])
		if pe == nil && p0.Status == "rejected" {
			return nil
		}
		p, e := s.LoadProposal(args[1])
		if e != nil {
			return e
		}
		p.Status = "rejected"
		p.UpdatedAt = evolution.Now()
		return s.SaveProposal(p)
	case "doctor":
		stats, e := s.Stats()
		if e != nil {
			return e
		}
		if *jsonOut {
			return emitEvolve(stats, true)
		}
		fmt.Printf("Evolution store: PASS (%s scope)\n", stats.ScopeID)
		for _, c := range stats.Collections {
			fmt.Printf("  %-12s files=%d bytes=%d earliest=%s latest=%s\n",
				c.Name, c.Files, c.Bytes, c.EarliestTime, c.LatestTime)
		}
		if len(stats.Snapshots) > 0 {
			fmt.Printf("  maintenance snapshots: %d\n", len(stats.Snapshots))
		}
		return nil
	case "prune":
		result, e := s.Prune(evolution.PruneOptions{KeepEpisodes: *keepEpisodes, DryRun: *dryRun})
		if e != nil {
			if *jsonOut {
				_ = emitEvolve(result, true)
			}
			return e
		}
		return emitEvolve(result, *jsonOut)
	case "repair":
		result, e := s.Repair(evolution.RepairOptions{DryRun: *dryRun})
		if e != nil {
			if *jsonOut {
				_ = emitEvolve(result, true)
			}
			return e
		}
		return emitEvolve(result, *jsonOut)
	case "rollback":
		if len(args) < 2 {
			return errors.New("rollback requires id")
		}
		if e := s.RestoreOverlay(args[1]); e != nil {
			return e
		}
		if e := runInstall([]string{"--project-dir", *project}, true); e != nil {
			return fmt.Errorf("restore overlay prompt: %w", e)
		}
		p, e := s.LoadProposal(args[1])
		if e == nil {
			p.Status = "rolled_back"
			p.UpdatedAt = evolution.Now()
			_ = s.SaveProposal(p)
		}
		return nil
	case "history":
		v, e := s.ListProposals()
		if e != nil {
			return e
		}
		if len(args) > 1 && args[1] != "--json" {
			filtered := v[:0]
			for _, proposal := range v {
				if proposal.ID == args[1] {
					filtered = append(filtered, proposal)
				}
			}
			v = filtered
		}
		return emitEvolve(v, *jsonOut)
	default:
		return fmt.Errorf("unknown evolve command %q", args[0])
	}
}
func emitEvolve(v any, asJSON bool) error {
	if asJSON {
		return writeJSONOutput(v)
	}
	b, _ := json.MarshalIndent(v, "", "  ")
	fmt.Println(string(b))
	return nil
}
