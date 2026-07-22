package config

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
)

// tomlRawConfig preserves raw TOML values without expanding environment variables.
type tomlRawConfig struct {
	Quality  map[string]string
	Runtime  map[string]string
	Agents   map[string]map[string]string // profile → key → raw value
	Routing  map[string]string
	Disabled []string // comma-separated disabled profiles (already split)
}

// parseTOMLRaw parses a TOML config file preserving raw values (no env expansion).
func parseTOMLRaw(path string) (*tomlRawConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw := &tomlRawConfig{}
	section := ""
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for lineNo := 1; scanner.Scan(); lineNo++ {
		line := strings.TrimSpace(stripComment(scanner.Text()))
		if line == "" {
			continue
		}
		if strings.HasPrefix(line, "[") && strings.HasSuffix(line, "]") {
			section = strings.TrimSpace(line[1 : len(line)-1])
			if section != "quality" && section != "runtime" && section != "routing" && section != "profiles" && !strings.HasPrefix(section, "agent.") {
				return nil, fmt.Errorf("%s:%d: unsupported section %q", path, lineNo, section)
			}
			continue
		}
		parts := strings.SplitN(line, "=", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("%s:%d: expected key = value", path, lineNo)
		}
		key := strings.TrimSpace(parts[0])
		rawVal := strings.TrimSpace(parts[1])
		rawValue := stringValue(rawVal)

		switch {
		case section == "quality":
			if raw.Quality == nil {
				raw.Quality = make(map[string]string)
			}
			raw.Quality[key] = rawValue
		case section == "runtime":
			if raw.Runtime == nil {
				raw.Runtime = make(map[string]string)
			}
			raw.Runtime[key] = rawValue
		case strings.HasPrefix(section, "agent."):
			profile := strings.TrimSpace(strings.TrimPrefix(section, "agent."))
			if raw.Agents == nil {
				raw.Agents = make(map[string]map[string]string)
			}
			if raw.Agents[profile] == nil {
				raw.Agents[profile] = make(map[string]string)
			}
			raw.Agents[profile][key] = rawValue
		case section == "routing":
			if raw.Routing == nil {
				raw.Routing = make(map[string]string)
			}
			raw.Routing[key] = rawValue
		case section == "profiles":
			if key == "disabled" {
				for _, p := range strings.Split(rawValue, ",") {
					p = strings.TrimSpace(p)
					if p != "" {
						raw.Disabled = append(raw.Disabled, p)
					}
				}
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return raw, nil
}

// toJSONValue converts a raw TOML value string to a JSON-compatible Go value.
func toJSONValue(raw string) interface{} {
	// Quoted string
	if (strings.HasPrefix(raw, `"`) && strings.HasSuffix(raw, `"`)) ||
		(strings.HasPrefix(raw, `'`) && strings.HasSuffix(raw, `'`)) {
		return raw[1 : len(raw)-1]
	}
	// Boolean
	if raw == "true" {
		return true
	}
	if raw == "false" {
		return false
	}
	// Integer
	if i, err := strconv.ParseInt(raw, 10, 64); err == nil {
		return i
	}
	// Float
	if f, err := strconv.ParseFloat(raw, 64); err == nil {
		return f
	}
	// Fallback: string
	return raw
}

// toJSONC converts the raw TOML config to formatted JSONC bytes.
func (r *tomlRawConfig) toJSONC() ([]byte, error) {
	var buf bytes.Buffer
	buf.WriteString("{\n")

	type section struct {
		comment string
		content string
	}

	var sections []section

	// quality
	if len(r.Quality) > 0 {
		var sb bytes.Buffer
		sb.WriteString("\t\"quality\": {\n")
		keys := sortedKeys(r.Quality)
		for i, k := range keys {
			writeJSONCValue(&sb, k, r.Quality[k], 2)
			if i < len(keys)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString("\t}")
		sections = append(sections, section{"Quality settings", sb.String()})
	}

	// runtime
	if len(r.Runtime) > 0 {
		var sb bytes.Buffer
		sb.WriteString("\t\"runtime\": {\n")
		keys := sortedKeys(r.Runtime)
		for i, k := range keys {
			writeJSONCValue(&sb, k, r.Runtime[k], 2)
			if i < len(keys)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString("\t}")
		sections = append(sections, section{"Runtime settings", sb.String()})
	}

	// agent
	if len(r.Agents) > 0 {
		var sb bytes.Buffer
		sb.WriteString("\t\"agent\": {\n")
		profiles := make([]string, 0, len(r.Agents))
		for p := range r.Agents {
			profiles = append(profiles, p)
		}
		sort.Strings(profiles)
		for pi, profile := range profiles {
			agentKeys := r.Agents[profile]
			fmt.Fprintf(&sb, "\t\t%q: {\n", profile)
			ks := sortedKeys(agentKeys)
			for j, k := range ks {
				writeJSONCValue(&sb, k, agentKeys[k], 3)
				if j < len(ks)-1 {
					sb.WriteByte(',')
				}
				sb.WriteByte('\n')
			}
			sb.WriteString("\t\t}")
			if pi < len(profiles)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString("\t}")
		sections = append(sections, section{"Agent overrides", sb.String()})
	}

	// routing
	if len(r.Routing) > 0 {
		var sb bytes.Buffer
		sb.WriteString("\t\"routing\": {\n")
		keys := sortedKeys(r.Routing)
		for i, k := range keys {
			fmt.Fprintf(&sb, "\t\t%q: %q", k, r.Routing[k])
			if i < len(keys)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString("\t}")
		sections = append(sections, section{"Category routing", sb.String()})
	}

	// profiles
	if len(r.Disabled) > 0 {
		var sb bytes.Buffer
		sb.WriteString("\t\"profiles\": {\n")
		sb.WriteString("\t\t\"disabled\": [\n")
		for i, p := range r.Disabled {
			fmt.Fprintf(&sb, "\t\t\t%q", p)
			if i < len(r.Disabled)-1 {
				sb.WriteByte(',')
			}
			sb.WriteByte('\n')
		}
		sb.WriteString("\t\t]\n")
		sb.WriteString("\t}")
		sections = append(sections, section{"Disabled profiles", sb.String()})
	}

	for i, sec := range sections {
		fmt.Fprintf(&buf, "\t// %s\n", sec.comment)
		buf.WriteString(sec.content)
		if i < len(sections)-1 {
			buf.WriteString(",\n\n")
		} else {
			buf.WriteByte('\n')
		}
	}

	buf.WriteString("}\n")
	return buf.Bytes(), nil
}

// writeJSONCValue writes a key-value pair as JSONC, inferring the type from the raw string.
func writeJSONCValue(buf *bytes.Buffer, key, raw string, indent int) {
	prefix := strings.Repeat("\t", indent)
	v := toJSONValue(raw)
	switch val := v.(type) {
	case string:
		fmt.Fprintf(buf, "%s%q: %q", prefix, key, val)
	case bool:
		fmt.Fprintf(buf, "%s%q: %t", prefix, key, val)
	case int64:
		fmt.Fprintf(buf, "%s%q: %d", prefix, key, val)
	case float64:
		fmt.Fprintf(buf, "%s%q: %g", prefix, key, val)
	default:
		fmt.Fprintf(buf, "%s%q: %q", prefix, key, raw)
	}
}

func sortedKeys(m map[string]string) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// MigratePlan describes what a migration would do.
type MigratePlan struct {
	SourcePath   string `json:"source_path"`
	DestPath     string `json:"dest_path"`
	BackupPath   string `json:"backup_path"`
	SourceExists bool   `json:"source_exists"`
	DestExists   bool   `json:"dest_exists"`
	CanMigrate   bool   `json:"can_migrate"`
	AlreadyDone  bool   `json:"already_done,omitempty"`
	Conflict     string `json:"conflict,omitempty"`
}

// PlanMigration returns a migration plan without writing any files.
func PlanMigration(sourcePath, destPath string) MigratePlan {
	plan := MigratePlan{
		SourcePath: sourcePath,
		DestPath:   destPath,
		BackupPath: sourcePath + ".bak",
	}

	if _, err := os.Stat(sourcePath); err != nil {
		plan.SourceExists = false
		plan.CanMigrate = false
		return plan
	}
	plan.SourceExists = true

	if _, err := os.Stat(destPath); err == nil {
		plan.DestExists = true
		// Check if already migrated (content equivalent)
		if configsEqual(destPath, sourcePath) {
			plan.AlreadyDone = true
			plan.CanMigrate = true
			return plan
		}
		plan.CanMigrate = false
		plan.Conflict = fmt.Sprintf("destination %q already exists with different content", destPath)
		return plan
	}

	plan.CanMigrate = true
	return plan
}

// ExecuteMigration converts a TOML config to JSONC, creating a backup of the original.
func ExecuteMigration(sourcePath, destPath string, force bool) error {
	// Ensure source exists
	if _, err := os.Stat(sourcePath); err != nil {
		return fmt.Errorf("source config not found: %s", sourcePath)
	}

	// Check destination conflict
	if !force {
		if _, err := os.Stat(destPath); err == nil {
			// Already migrated?
			if configsEqual(destPath, sourcePath) {
				return nil // idempotent: already up-to-date
			}
			return fmt.Errorf("destination %q already exists with different content (use --force to overwrite)", destPath)
		}
	}

	// Parse TOML preserving raw values
	raw, err := parseTOMLRaw(sourcePath)
	if err != nil {
		return fmt.Errorf("parse source: %w", err)
	}

	// Generate JSONC
	jsoncData, err := raw.toJSONC()
	if err != nil {
		return fmt.Errorf("generate JSONC: %w", err)
	}

	// Create backup of source
	backupPath := sourcePath + ".bak"
	origData, err := os.ReadFile(sourcePath)
	if err != nil {
		return fmt.Errorf("read source for backup: %w", err)
	}
	if err := os.WriteFile(backupPath, origData, 0o600); err != nil {
		return fmt.Errorf("create backup: %w", err)
	}

	// Ensure destination directory exists
	destDir := filepath.Dir(destPath)
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return fmt.Errorf("create destination directory: %w", err)
	}

	// Write JSONC
	if err := os.WriteFile(destPath, jsoncData, 0o600); err != nil {
		return fmt.Errorf("write JSONC: %w", err)
	}

	// Validate: load both and compare
	srcCfg, srcErr := loadTOML(sourcePath)
	dstCfg, dstErr := loadJSONC(destPath)
	if srcErr != nil {
		return fmt.Errorf("post-migration source validation: %w", srcErr)
	}
	if dstErr != nil {
		return fmt.Errorf("post-migration destination validation: %w", dstErr)
	}

	// Compare equivalence
	diff := configDiff(srcCfg, dstCfg)
	if diff != "" {
		// Rollback
		os.Remove(destPath)
		os.Rename(backupPath, sourcePath)
		return fmt.Errorf("migration validation failed: %s", diff)
	}

	return nil
}

// configsEqual checks if a JSONC config file and a TOML config file produce the same Config.
func configsEqual(jsoncPath, tomlPath string) bool {
	dstCfg, dstErr := loadJSONC(jsoncPath)
	if dstErr != nil {
		return false
	}
	srcCfg, srcErr := loadTOML(tomlPath)
	if srcErr != nil {
		return false
	}
	return configDiff(srcCfg, dstCfg) == ""
}

// configDiff returns a description of differences, or "" if equivalent.
func configDiff(a, b Config) string {
	var diffs []string

	if a.Fixtures != b.Fixtures {
		diffs = append(diffs, fmt.Sprintf("Fixtures: %q != %q", a.Fixtures, b.Fixtures))
	}
	if a.MetricsDir != b.MetricsDir {
		diffs = append(diffs, fmt.Sprintf("MetricsDir: %q != %q", a.MetricsDir, b.MetricsDir))
	}
	if a.Model != b.Model {
		diffs = append(diffs, fmt.Sprintf("Model: %q != %q", a.Model, b.Model))
	}
	if a.MaxSteps != b.MaxSteps {
		diffs = append(diffs, fmt.Sprintf("MaxSteps: %d != %d", a.MaxSteps, b.MaxSteps))
	}
	if a.Concurrency != b.Concurrency {
		diffs = append(diffs, fmt.Sprintf("Concurrency: %d != %d", a.Concurrency, b.Concurrency))
	}
	if a.Timeout != b.Timeout {
		diffs = append(diffs, fmt.Sprintf("Timeout: %s != %s", a.Timeout, b.Timeout))
	}
	if a.MinQualifiedRate != b.MinQualifiedRate {
		diffs = append(diffs, fmt.Sprintf("MinQualifiedRate: %f != %f", a.MinQualifiedRate, b.MinQualifiedRate))
	}
	if a.MaxCost != b.MaxCost {
		diffs = append(diffs, fmt.Sprintf("MaxCost: %f != %f", a.MaxCost, b.MaxCost))
	}

	// Compare agents map
	agentKeys := make(map[string]bool)
	for k := range a.Agents {
		agentKeys[k] = true
	}
	for k := range b.Agents {
		agentKeys[k] = true
	}
	for k := range agentKeys {
		aa, aOk := a.Agents[k]
		bb, bOk := b.Agents[k]
		if aOk != bOk {
			diffs = append(diffs, fmt.Sprintf("agent %q: exists_a=%v, exists_b=%v", k, aOk, bOk))
			continue
		}
		if aa.Model != bb.Model {
			diffs = append(diffs, fmt.Sprintf("agent %q model: %q != %q", k, aa.Model, bb.Model))
		}
		if aa.PromptFile != bb.PromptFile {
			diffs = append(diffs, fmt.Sprintf("agent %q prompt_file: %q != %q", k, aa.PromptFile, bb.PromptFile))
		}
		if (aa.ReadOnly == nil) != (bb.ReadOnly == nil) {
			diffs = append(diffs, fmt.Sprintf("agent %q read_only: set=%v != set=%v", k, aa.ReadOnly != nil, bb.ReadOnly != nil))
		} else if aa.ReadOnly != nil && bb.ReadOnly != nil && *aa.ReadOnly != *bb.ReadOnly {
			diffs = append(diffs, fmt.Sprintf("agent %q read_only: %t != %t", k, *aa.ReadOnly, *bb.ReadOnly))
		}
	}

	// Compare categories (routing)
	catKeys := make(map[string]bool)
	for k := range a.Categories {
		catKeys[k] = true
	}
	for k := range b.Categories {
		catKeys[k] = true
	}
	for k := range catKeys {
		if a.Categories[k] != b.Categories[k] {
			diffs = append(diffs, fmt.Sprintf("routing %q: %q != %q", k, a.Categories[k], b.Categories[k]))
		}
	}

	// Compare disabled profiles (sorted)
	sort.Strings(a.DisabledProfiles)
	sort.Strings(b.DisabledProfiles)
	if len(a.DisabledProfiles) != len(b.DisabledProfiles) {
		diffs = append(diffs, fmt.Sprintf("disabled profiles: %v != %v", a.DisabledProfiles, b.DisabledProfiles))
	} else {
		for i := range a.DisabledProfiles {
			if a.DisabledProfiles[i] != b.DisabledProfiles[i] {
				diffs = append(diffs, fmt.Sprintf("disabled profiles: %v != %v", a.DisabledProfiles, b.DisabledProfiles))
				break
			}
		}
	}

	if len(diffs) > 0 {
		return strings.Join(diffs, "; ")
	}
	return ""
}

// DefaultConfigPaths returns the default source (.toml) and destination (.jsonc) paths.
func DefaultConfigPaths(root string) (source, dest string) {
	base := filepath.Join(root, ".reasonix", "omr")
	source = filepath.Join(base, "config.toml")
	dest = filepath.Join(base, "config.jsonc")
	return
}
