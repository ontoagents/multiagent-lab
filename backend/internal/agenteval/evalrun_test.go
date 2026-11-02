package agenteval

// REQ-388/M387 C4 EvaluateRun 单测：LLM 桩注入（runJudgeLLM）——prompt 组装+输出解析。

import (
	"context"
	"strings"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/secrets"
	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func TestEvaluateRunPromptAndParse(t *testing.T) {
	orig := runJudgeLLM
	t.Cleanup(func() { runJudgeLLM = orig })
	var captured string
	runJudgeLLM = func(ctx context.Context, st *store.Store, box *secrets.Box, connID, system, user string, temperature *float64) (string, error) {
		captured = system + "\n" + user
		return `{"score":2,"verdict":"pass","reason":"目标达成且过程合理"}`, nil
	}
	res, err := EvaluateRun(context.Background(), nil, nil, "", RunEvalInput{
		Goal: "查北京天气", Answer: "北京今日晴 25 度", Tools: []string{"weather"}, Warnings: []string{"限流一次"},
	})
	if err != nil {
		t.Fatal(err)
	}
	if res.Score != 2 || res.Verdict != "pass" {
		t.Fatalf("解析不符: %+v", res)
	}
	for _, want := range []string{"运行质量评估器", "查北京天气", "北京今日晴 25 度", "- weather", "限流一次"} {
		if !strings.Contains(captured, want) {
			t.Fatalf("prompt 缺 %q:\n%s", want, captured)
		}
	}
}
