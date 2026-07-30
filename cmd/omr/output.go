package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
)

// normalizeLeadingTargetArgs accepts the documented "<target> [flags]" form
// even though Go's flag package stops parsing at the first positional argument.
func normalizeLeadingTargetArgs(args []string) []string {
	if len(args) < 2 || strings.HasPrefix(args[0], "-") {
		return args
	}
	normalized := append([]string(nil), args[1:]...)
	return append(normalized, args[0])
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

func encodeJSON(writer io.Writer, value any, pretty bool) error {
	encoder := json.NewEncoder(writer)
	if pretty {
		encoder.SetIndent("", "  ")
	}
	return encoder.Encode(value)
}

func writeJSONOutput(value any) error {
	return encodeJSON(os.Stdout, value, false)
}

func writePrettyJSONOutput(value any) error {
	return encodeJSON(os.Stdout, value, true)
}

func writeJSONValue(path string, label string, value interface{}) error {
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
	if err := encodeJSON(writer, value, true); err != nil {
		return err
	}
	if path != "" {
		fmt.Printf("%s report: %s\n", label, path)
	}
	return nil
}
