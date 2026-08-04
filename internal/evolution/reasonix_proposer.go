package evolution

import (
	"context"
	"fmt"
	"github.com/mchenziyi/oh-my-reasonix/internal/reasonix"
)

type ReasonixProposer struct{ Runner reasonix.Runner }

func (p ReasonixProposer) Propose(pattern Pattern) (Proposal, error) {
	prompt := fmt.Sprintf("输出严格 JSON Proposal，不要修改文件。pattern=%s failure=%s episodes=%v。字段：schema_version,id,pattern_id,title,rationale,overlay,status,content_sha256,created_at,updated_at。overlay 只能是项目策略文本。", pattern.TaskClass, pattern.FailureClass, pattern.EpisodeIDs)
	r := p.Runner.RunTask(context.Background(), reasonix.TaskOptions{Prompt: prompt})
	if r.Err != nil {
		return Proposal{}, r.Err
	}
	return ParseProposal([]byte(r.Stdout))
}
