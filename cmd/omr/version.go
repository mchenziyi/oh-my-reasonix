package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
)

var version = "2.0.0"

const minimumReasonixVersion = "1.17.20"

func runVersion(args []string) error {
	flags := flag.NewFlagSet("version", flag.ContinueOnError)
	flags.SetOutput(os.Stderr)
	jsonOutput := flags.Bool("json", false, "output version info as JSON")
	if err := flags.Parse(args); err != nil {
		return err
	}
	if *jsonOutput {
		type versionInfo struct {
			OMR             string `json:"omr_version"`
			Manifest        string `json:"manifest_schema"`
			Assets          string `json:"assets_version"`
			MinimumReasonix string `json:"minimum_reasonix_version"`
			Reasonix        string `json:"reasonix_detected"`
			Compatible      bool   `json:"compatible"`
		}
		info := versionInfo{OMR: version, Manifest: "1", Assets: "builtin", MinimumReasonix: minimumReasonixVersion, Compatible: true}
		if path, lookErr := exec.LookPath("reasonix"); lookErr == nil {
			if data, execErr := exec.Command(path, "version").Output(); execErr == nil {
				info.Reasonix = strings.TrimSpace(string(data))
				info.Compatible = compatibleReasonixVersion(info.Reasonix)
			} else {
				info.Reasonix = "detected but version check failed"
				info.Compatible = false
			}
		} else {
			info.Reasonix = "not found in PATH"
			info.Compatible = false
		}
		return writeJSONOutput(info)
	}
	fmt.Printf("omr %s\n", version)
	if path, lookErr := exec.LookPath("reasonix"); lookErr == nil {
		fmt.Printf("reasonix: %s\n", path)
	} else {
		fmt.Println("reasonix: not found in PATH")
	}
	return nil
}

func compatibleReasonixVersion(output string) bool {
	for _, field := range strings.Fields(output) {
		field = strings.TrimPrefix(field, "v")
		parts := strings.Split(field, ".")
		if len(parts) != 3 {
			continue
		}
		values := make([]int, 3)
		valid := true
		for i, part := range parts {
			value, err := strconv.Atoi(part)
			if err != nil {
				valid = false
				break
			}
			values[i] = value
		}
		if !valid {
			continue
		}
		minimum := strings.Split(minimumReasonixVersion, ".")
		for i := range values {
			min, _ := strconv.Atoi(minimum[i])
			if values[i] != min {
				return values[i] > min
			}
		}
		return true
	}
	return false
}
