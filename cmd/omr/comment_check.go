package main

import (
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/mchenziyi/oh-my-reasonix/internal/commentchecker"
)

func runCommentCheck(args []string) error {
	flags := flag.NewFlagSet("comment-check", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	projectDir := flags.String("project-dir", ".", "project root directory")
	filePath := flags.String("path", "", "specific file or directory to check (default: scan project)")
	jsonOutput := flags.Bool("json", false, "output report as JSON")
	maxFileSize := flags.Int64("max-file-size", 1<<20, "maximum file size in bytes (files larger are skipped)")
	allowTags := flags.String("allow-tags", "", "comma-separated list of allowed TODO/FIXME tags (e.g. \"TODO(admin),TODO(future)\")")
	allowedRoots := flags.String("allowed-roots", "", "comma-separated list of allowed root directories")
	if err := flags.Parse(args); err != nil {
		return err
	}

	cfg := commentchecker.Config{MaxFileSize: *maxFileSize}
	if *allowTags != "" {
		for _, tag := range strings.Split(*allowTags, ",") {
			tag = strings.TrimSpace(tag)
			if tag != "" {
				cfg.AllowedTags = append(cfg.AllowedTags, tag)
			}
		}
	}
	if *allowedRoots != "" {
		for _, root := range strings.Split(*allowedRoots, ",") {
			root = strings.TrimSpace(root)
			if root != "" {
				cfg.AllowedRoots = append(cfg.AllowedRoots, root)
			}
		}
	}
	if *filePath != "" {
		// Resolve relative --path against --project-dir so that
		// "omr comment-check --project-dir . --path foo.go" always
		// refers to <project-dir>/foo.go regardless of the shell cwd.
		resolved := *filePath
		if !filepath.IsAbs(resolved) {
			resolved = filepath.Join(*projectDir, resolved)
		}
		cfg.Files = []string{filepath.Clean(resolved)}
	}

	report, err := commentchecker.Run(*projectDir, cfg)
	if err != nil {
		return err
	}

	if *jsonOutput {
		if err := writePrettyJSONOutput(report); err != nil {
			return err
		}
	} else {
		fmt.Print(report.HumanString())
	}

	if report.BlockingCount > 0 {
		return fmt.Errorf("%d blocking finding(s) found", report.BlockingCount)
	}
	return nil
}
