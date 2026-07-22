package reasonix

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// sessionStore is the path to the Reasonix global session store.
// Relative to user's home directory.
const sessionStoreRel = ".reasonix"

// SessionInfo represents a Reasonix session from the filesystem.
type SessionInfo struct {
	ID          string       `json:"id"`
	ProjectPath string       `json:"project_path"`
	UpdatedAt   string       `json:"updated_at"`
	GoalState   *GoalState   `json:"goal_state,omitempty"`
	HasGoalFile bool         `json:"has_goal_file"`
}

// GoalState mirrors the Reasonix goal-state.json structure.
type GoalState struct {
	Goal             string          `json:"goal"`
	Status           string          `json:"status"`
	ScopeID          string          `json:"scopeID"`
	Turns            int             `json:"turns"`
	Blocks           int             `json:"blocks"`
	Block            string          `json:"block,omitempty"`
	Todos            []GoalTodo      `json:"todos,omitempty"`
}

// GoalTodo represents a single todo item in a goal state.
type GoalTodo struct {
	Content    string `json:"content"`
	Status     string `json:"status"`
	ActiveForm string `json:"activeForm,omitempty"`
}

// ListSessions reads all Reasonix session files from the global store
// and returns a summary for each session. Read-only: never writes.
func ListSessions() ([]SessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("home dir: %w", err)
	}
	projectsDir := filepath.Join(home, sessionStoreRel, "projects")
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return nil, fmt.Errorf("read sessions dir: %w", err)
	}

	var sessions []SessionInfo
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		sessionsDir := filepath.Join(projectsDir, entry.Name(), "sessions")
		sessionEntries, err := os.ReadDir(sessionsDir)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			continue
		}

		for _, se := range sessionEntries {
			if se.IsDir() || !strings.HasSuffix(se.Name(), "-session.goal-state.json") {
				continue
			}
			sessionID := strings.TrimSuffix(se.Name(), "-session.goal-state.json")

			info := SessionInfo{
				ID:          sessionID,
				ProjectPath: resolveProjectPath(projectsDir, entry.Name()),
				HasGoalFile: true,
			}

			// Read goal state
			goalPath := filepath.Join(sessionsDir, se.Name())
			data, err := os.ReadFile(goalPath)
			if err == nil {
				var gs GoalState
				if json.Unmarshal(data, &gs) == nil {
					info.GoalState = &gs
					info.UpdatedAt = gs.ScopeID
				}
			}

			// Try to get updated_at from the .jsonl.meta or event-index
			metaFiles, _ := filepath.Glob(filepath.Join(sessionsDir, sessionID+"-session.jsonl.*"))
			for _, mf := range metaFiles {
				if strings.HasSuffix(mf, ".meta") {
					if metaData, err := os.ReadFile(mf); err == nil {
						info.UpdatedAt = strings.TrimSpace(string(metaData))
					}
				}
			}

			sessions = append(sessions, info)
		}
	}

	sort.Slice(sessions, func(i, j int) bool {
		return sessions[i].ID > sessions[j].ID
	})

	return sessions, nil
}

// ReadSessionStatus reads the goal-state.json for a specific session.
func ReadSessionStatus(sessionID string) (*SessionInfo, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil, err
	}

	// Search all project directories for this session
	projectsDir := filepath.Join(home, sessionStoreRel, "projects")
	entries, _ := os.ReadDir(projectsDir)

	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		pattern := filepath.Join(projectsDir, entry.Name(), "sessions", sessionID+"-session.goal-state.json")
		matches, _ := filepath.Glob(pattern)
		for _, goalPath := range matches {
			info := SessionInfo{
				ID:          sessionID,
				ProjectPath: resolveProjectPath(projectsDir, entry.Name()),
				HasGoalFile: true,
			}
			data, err := os.ReadFile(goalPath)
			if err == nil {
				var gs GoalState
				if json.Unmarshal(data, &gs) == nil {
					info.GoalState = &gs
				}
			}
			return &info, nil
		}
	}

	return nil, fmt.Errorf("session %q not found", sessionID)
}

// resolveProjectPath tries to read the actual project path from the session
// meta file, falling back to heuristic decoding of the directory name.
func resolveProjectPath(projectsDir, dirName string) string {
	// Try to read a session meta file to get the real workspace_root
	sessionsDir := filepath.Join(projectsDir, dirName, "sessions")
	metaFiles, err := filepath.Glob(filepath.Join(sessionsDir, "*-session.jsonl.meta"))
	if err == nil && len(metaFiles) > 0 {
		data, err := os.ReadFile(metaFiles[0])
		if err == nil {
			var meta struct {
				WorkspaceRoot string `json:"workspace_root"`
			}
			if json.Unmarshal(data, &meta) == nil && meta.WorkspaceRoot != "" {
				return meta.WorkspaceRoot
			}
		}
	}
	return decodeProjectDirHeuristic(dirName)
}

// decodeProjectDirHeuristic attempts to decode the encoded project directory name.
func decodeProjectDirHeuristic(encoded string) string {
	if strings.HasPrefix(encoded, "-private-tmp-") {
		return "/tmp/" + strings.ReplaceAll(encoded[13:], "-", "/")
	}
	if strings.HasPrefix(encoded, "-") {
		return "/" + strings.ReplaceAll(encoded[1:], "-", "/")
	}
	return encoded
}
