package config

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"time"
)

// stripJSONCComments removes // and /* */ comments from JSONC text.
func stripJSONCComments(raw []byte) ([]byte, []int, error) {
	var lineStarts []int
	lineStarts = append(lineStarts, 0)
	for i, b := range raw {
		if b == '\n' {
			lineStarts = append(lineStarts, i+1)
		}
	}

	// posToLineCol converts a byte offset to 1-based line/col
	posToLineCol := func(pos int) (int, int) {
		line := 0
		for line+1 < len(lineStarts) && lineStarts[line+1] <= pos {
			line++
		}
		return line + 1, pos - lineStarts[line] + 1
	}

	var out []byte
	inString := false
	inLineComment := false
	inBlockComment := false
	var quoteByte byte

	for i := 0; i < len(raw); i++ {
		b := raw[i]

		if inLineComment {
			if b == '\n' {
				inLineComment = false
				out = append(out, b)
			}
			continue
		}
		if inBlockComment {
			if b == '*' && i+1 < len(raw) && raw[i+1] == '/' {
				inBlockComment = false
				i++ // skip '/'
			}
			continue
		}
		if inString {
			out = append(out, b)
			if b == '\\' && i+1 < len(raw) {
				i++
				out = append(out, raw[i])
			} else if b == quoteByte {
				inString = false
			}
			continue
		}
		// Not in string or comment
		if b == '"' || b == '\'' {
			inString = true
			quoteByte = b
			out = append(out, b)
			continue
		}
		if b == '/' && i+1 < len(raw) {
			if raw[i+1] == '/' {
				inLineComment = true
				i++ // skip second '/'
				continue
			}
			if raw[i+1] == '*' {
				inBlockComment = true
				i++ // skip '*'
				continue
			}
		}
		out = append(out, b)
	}

	if inBlockComment {
		line, col := posToLineCol(len(raw) - 1)
		return nil, nil, fmt.Errorf("unterminated block comment at line %d, col %d", line, col)
	}

	// Build line-start index for the output (used by JSON decoder error position)
	return out, lineStarts, nil
}

// jsoncRawConfig mirrors the JSONC config structure for intermediate unmarshal.
type jsoncRawConfig struct {
	Quality  *jsoncQuality         `json:"quality,omitempty"`
	Runtime  *jsoncRuntime         `json:"runtime,omitempty"`
	Agent    map[string]jsoncAgent `json:"agent,omitempty"`
	Routing  map[string]string     `json:"routing,omitempty"`
	Profiles *jsoncProfiles        `json:"profiles,omitempty"`
	MCP      map[string]jsoncMCP   `json:"mcp,omitempty"`
}

type jsoncMCP struct {
	Transport    string   `json:"transport,omitempty"`
	Command      string   `json:"command,omitempty"`
	Args         []string `json:"args,omitempty"`
	URL          string   `json:"url,omitempty"`
	Capabilities []string `json:"capabilities,omitempty"`
	Enabled      *bool    `json:"enabled,omitempty"`
	Env          []string `json:"env,omitempty"`
}

type jsoncQuality struct {
	Fixtures         string   `json:"fixtures,omitempty"`
	MinQualifiedRate *float64 `json:"min_qualified_rate,omitempty"`
	MaxCost          *float64 `json:"max_cost,omitempty"`
}

type jsoncRuntime struct {
	MetricsDir  string `json:"metrics_dir,omitempty"`
	Model       string `json:"model,omitempty"`
	MaxSteps    int    `json:"max_steps,omitempty"`
	Concurrency int    `json:"concurrency,omitempty"`
	Timeout     string `json:"timeout,omitempty"`
}

type jsoncAgent struct {
	Model      string `json:"model,omitempty"`
	PromptFile string `json:"prompt_file,omitempty"`
	ReadOnly   *bool  `json:"read_only,omitempty"`
}

type jsoncProfiles struct {
	Disabled []string `json:"disabled,omitempty"`
}

// loadJSONC parses a JSONC config file and returns a Config.
func loadJSONC(path string) (Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}

	// Check for BOM
	clean := raw
	if len(clean) >= 3 && clean[0] == 0xEF && clean[1] == 0xBB && clean[2] == 0xBF {
		clean = clean[3:]
	}

	stripped, _, err := stripJSONCComments(clean)
	if err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}

	// Build line-start index from stripped text for JSON error position mapping
	strippedLineStarts := []int{0}
	for i, b := range stripped {
		if b == '\n' {
			strippedLineStarts = append(strippedLineStarts, i+1)
		}
	}

	var rawCfg jsoncRawConfig
	decoder := json.NewDecoder(strings.NewReader(string(stripped)))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(&rawCfg); err != nil {
		// Try to extract position from JSON syntax error
		if syntaxErr, ok := err.(*json.SyntaxError); ok {
			if syntaxErr.Offset > 0 {
				offset := int(syntaxErr.Offset)
				line, col := offsetToLineCol(offset, strippedLineStarts, stripped)
				return Config{}, fmt.Errorf("%s:%d:%d: JSON syntax error: %v", path, line, col, syntaxErr.Error())
			}
			return Config{}, fmt.Errorf("%s: JSON syntax error: %v", path, syntaxErr.Error())
		}
		if unmarshalErr, ok := err.(*json.UnmarshalTypeError); ok {
			line, col := offsetToLineCol(int(unmarshalErr.Offset), strippedLineStarts, stripped)
			return Config{}, fmt.Errorf("%s:%d:%d: type error for field %q: expected %s, got %s",
				path, line, col, unmarshalErr.Field, unmarshalErr.Type.String(), unmarshalErr.Value)
		}
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}

	// Reject multiple JSON documents (e.g., two JSON objects in one file)
	var trailing json.RawMessage
	if err := decoder.Decode(&trailing); err == nil {
		return Config{}, fmt.Errorf("%s: JSON document contains multiple objects (only one allowed)", path)
	} else if err.Error() != "EOF" {
		return Config{}, fmt.Errorf("%s: JSON error after main document: %v", path, err)
	}

	// Check for duplicate keys by re-parsing into raw map
	if dupKey, dupOffset := detectDuplicateKeys(stripped); dupKey != "" {
		line, col := offsetToLineCol(dupOffset, strippedLineStarts, stripped)
		return Config{}, fmt.Errorf("%s:%d:%d: duplicate key %s", path, line, col, dupKey)
	}

	var cfg Config
	cfg.MinQualifiedRateSet = false
	cfg.MaxCostSet = false
	cfg.TimeoutSet = false

	// Track seen keys for duplicate detection within each section
	seen := make(map[string]bool)

	if rawCfg.Quality != nil {
		q := rawCfg.Quality
		if q.Fixtures != "" {
			if seen["quality.fixtures"] {
				return Config{}, fmt.Errorf("%s: duplicate key %q", path, "quality.fixtures")
			}
			seen["quality.fixtures"] = true
			expanded, err := expandEnv(q.Fixtures)
			if err != nil {
				return Config{}, fmt.Errorf("%s: quality.fixtures: %w", path, err)
			}
			cfg.Fixtures = expanded
		}
		if q.MinQualifiedRate != nil {
			seen["quality.min_qualified_rate"] = true
			if *q.MinQualifiedRate < 0 || *q.MinQualifiedRate > 1 {
				return Config{}, fmt.Errorf("%s: quality.min_qualified_rate must be in [0, 1], got %f", path, *q.MinQualifiedRate)
			}
			cfg.MinQualifiedRate = *q.MinQualifiedRate
			cfg.MinQualifiedRateSet = true
		}
		if q.MaxCost != nil {
			seen["quality.max_cost"] = true
			if *q.MaxCost < 0 {
				return Config{}, fmt.Errorf("%s: quality.max_cost must be >= 0, got %f", path, *q.MaxCost)
			}
			cfg.MaxCost = *q.MaxCost
			cfg.MaxCostSet = true
		}
	}

	if rawCfg.Runtime != nil {
		r := rawCfg.Runtime
		if r.MetricsDir != "" {
			expanded, err := expandEnv(r.MetricsDir)
			if err != nil {
				return Config{}, fmt.Errorf("%s: runtime.metrics_dir: %w", path, err)
			}
			cfg.MetricsDir = expanded
		}
		if r.Model != "" {
			expanded, err := expandEnv(r.Model)
			if err != nil {
				return Config{}, fmt.Errorf("%s: runtime.model: %w", path, err)
			}
			cfg.Model = expanded
		}
		if r.MaxSteps != 0 {
			if r.MaxSteps < 0 {
				return Config{}, fmt.Errorf("%s: runtime.max_steps must be >= 0, got %d", path, r.MaxSteps)
			}
			cfg.MaxSteps = r.MaxSteps
		}
		if r.Concurrency != 0 {
			if r.Concurrency < 0 {
				return Config{}, fmt.Errorf("%s: runtime.concurrency must be >= 0, got %d", path, r.Concurrency)
			}
			cfg.Concurrency = r.Concurrency
		}
		if r.Timeout != "" {
			d, err := time.ParseDuration(r.Timeout)
			if err != nil {
				return Config{}, fmt.Errorf("%s: runtime.timeout: %w", path, err)
			}
			if d < 0 {
				return Config{}, fmt.Errorf("%s: runtime.timeout must be >= 0", path)
			}
			cfg.Timeout = d
			cfg.TimeoutSet = true
		}
	}

	if rawCfg.Agent != nil {
		for profileName, agent := range rawCfg.Agent {
			if strings.TrimSpace(profileName) != profileName || strings.ContainsAny(profileName, " \t/\\") {
				return Config{}, fmt.Errorf("%s: invalid agent profile name %q", path, profileName)
			}
			if cfg.Agents == nil {
				cfg.Agents = make(map[string]AgentConfig)
			}
			a := AgentConfig{}
			if agent.Model != "" {
				expanded, err := expandEnv(agent.Model)
				if err != nil {
					return Config{}, fmt.Errorf("%s: agent.%s.model: %w", path, profileName, err)
				}
				if strings.ContainsAny(expanded, "\r\n\t") {
					return Config{}, fmt.Errorf("%s: invalid model for agent %q", path, profileName)
				}
				a.Model = expanded
			}
			if agent.PromptFile != "" {
				expanded, err := expandEnv(agent.PromptFile)
				if err != nil {
					return Config{}, fmt.Errorf("%s: agent.%s.prompt_file: %w", path, profileName, err)
				}
				if strings.HasPrefix(expanded, "/") || strings.Contains(expanded, "\\") || strings.Contains(expanded, "..") {
					return Config{}, fmt.Errorf("%s: agent.%s.prompt_file must be a project-relative path", path, profileName)
				}
				a.PromptFile = expanded
			}
			if agent.ReadOnly != nil {
				a.ReadOnly = agent.ReadOnly
			}
			cfg.Agents[profileName] = a
		}
	}

	if rawCfg.Routing != nil {
		for category, profile := range rawCfg.Routing {
			if category == "" || strings.ContainsAny(category, " \t/\\") {
				return Config{}, fmt.Errorf("%s: routing: invalid category %q", path, category)
			}
			if category != strings.ToLower(category) {
				return Config{}, fmt.Errorf("%s: routing: category %q must be lowercase", path, category)
			}
			if profile == "" || strings.ContainsAny(profile, "\r\n\t /\\") {
				return Config{}, fmt.Errorf("%s: routing: invalid profile for category %q", path, category)
			}
			profile = strings.ToLower(profile)
			if cfg.Categories == nil {
				cfg.Categories = make(map[string]string)
			}
			cfg.Categories[category] = profile
		}
	}

	if rawCfg.Profiles != nil && rawCfg.Profiles.Disabled != nil {
		seenProfiles := make(map[string]bool)
		for _, profile := range rawCfg.Profiles.Disabled {
			p := strings.ToLower(strings.TrimSpace(profile))
			if p == "" || strings.ContainsAny(p, " \t/\\") {
				return Config{}, fmt.Errorf("%s: invalid disabled Profile %q", path, profile)
			}
			if !seenProfiles[p] {
				seenProfiles[p] = true
				cfg.DisabledProfiles = append(cfg.DisabledProfiles, p)
			}
		}
	}

	if rawCfg.MCP != nil {
		for name, srv := range rawCfg.MCP {
			if name == "" || strings.ContainsAny(name, " \t/\\") {
				return Config{}, fmt.Errorf("%s: mcp: invalid server name %q", path, name)
			}
			if srv.Transport != "" && srv.Transport != "stdio" && srv.Transport != "http" {
				return Config{}, fmt.Errorf("%s: mcp.%s.transport must be 'stdio' or 'http'", path, name)
			}
			if srv.Transport == "stdio" && srv.Command == "" {
				return Config{}, fmt.Errorf("%s: mcp.%s: transport stdio requires command", path, name)
			}
			if srv.Transport == "http" && srv.URL == "" {
				return Config{}, fmt.Errorf("%s: mcp.%s: transport http requires url", path, name)
			}
			if cfg.MCPServers == nil {
				cfg.MCPServers = make(map[string]MCPServerConfig)
			}
			server := MCPServerConfig{
				Transport:    srv.Transport,
				Command:      srv.Command,
				Args:         append([]string(nil), srv.Args...),
				URL:          srv.URL,
				Capabilities: append([]string(nil), srv.Capabilities...),
				Env:          append([]string(nil), srv.Env...),
			}
			if srv.Enabled != nil {
				server.Enabled = *srv.Enabled
			}
			cfg.MCPServers[name] = server
		}
	}

	// Post-load validation (same as TOML parser)
	if cfg.MaxSteps < 0 || cfg.Concurrency < 0 || cfg.MinQualifiedRate < 0 || cfg.MinQualifiedRate > 1 || cfg.MaxCost < 0 || cfg.Timeout < 0 {
		return Config{}, fmt.Errorf("%s: invalid OMR benchmark configuration", path)
	}

	return cfg, nil
}

// detectDuplicateKeys scans stripped JSON text for duplicate keys at all
// nesting levels. Uses a depth-aware scanner to avoid cross-object false positives.
func detectDuplicateKeys(stripped []byte) (string, int) {
	text := string(stripped)
	inString := false
	var quoteByte byte
	depth := 0
	// Collect all quoted strings that are followed by ':' at each depth
	type keyPos struct {
		key    string
		offset int
	}
	keysAtDepth := map[int][]keyPos{}
	keyStart := -1

	for i := 0; i < len(text); i++ {
		ch := text[i]
		if inString {
			if ch == '\\' {
				i++ // skip escaped char
				continue
			}
			if ch == quoteByte {
				inString = false
				// Check if this string is a key (followed by ':')
				j := i + 1
				for j < len(text) && text[j] <= ' ' {
					j++
				}
				if j < len(text) && text[j] == ':' {
					key := text[keyStart : i+1]
					keysAtDepth[depth] = append(keysAtDepth[depth], keyPos{key, keyStart})
				}
				keyStart = -1
			}
			continue
		}
		if ch == '{' {
			depth++
			continue
		}
		if ch == '}' {
			// Check for duplicates at this depth before decrementing
			if keys, ok := keysAtDepth[depth]; ok && len(keys) > 1 {
				seen := make(map[string]int)
				for _, kp := range keys {
					if first, ok := seen[kp.key]; ok {
						return kp.key, first
					}
					seen[kp.key] = kp.offset
				}
			}
			delete(keysAtDepth, depth)
			depth--
			continue
		}
		if ch == '"' {
			inString = true
			quoteByte = '"'
			keyStart = i
			continue
		}
	}
	return "", -1
}

// offsetToLineCol converts a byte offset (in the original raw bytes with comments)
// to 1-based line/col using lineStarts from the original.
func offsetToLineCol(offset int, origLineStarts []int, raw []byte) (int, int) {
	// Build line starts for raw if needed
	var lineStarts []int
	if len(origLineStarts) > 1 || (len(origLineStarts) == 1 && len(raw) > 0) {
		// Use the provided lineStarts (from the original raw)
		lineStarts = origLineStarts
	} else {
		lineStarts = []int{0}
		for i, b := range raw {
			if b == '\n' {
				lineStarts = append(lineStarts, i+1)
			}
		}
	}
	line := 0
	for line+1 < len(lineStarts) && lineStarts[line+1] <= offset {
		line++
	}
	return line + 1, offset - lineStarts[line] + 1
}
