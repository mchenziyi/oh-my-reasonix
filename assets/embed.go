// Package assets exposes the release assets embedded in the standalone OMR
// binary. The repository files remain the source of truth for local
// development; go:embed makes the same assets available after downloading a
// release binary.
package assets

import _ "embed"

//go:embed prompts/reasonix-base-464d494.md
var BasePrompt []byte

//go:embed prompts/orchestrator.zh.md
var Orchestrator []byte

//go:embed prompts/review-task-protocol.zh.md
var ReviewBrief []byte

//go:embed skills/omr-explore/SKILL.md
var Explore []byte

//go:embed skills/omr-research/SKILL.md
var Research []byte

//go:embed skills/omr-debug/SKILL.md
var Debug []byte

//go:embed skills/omr-planner/SKILL.md
var Planner []byte

//go:embed skills/omr-frontend/SKILL.md
var Frontend []byte
