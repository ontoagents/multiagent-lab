// REQ-450 单测：安装引导随 Status 透出——外部后端有引导（命令型/说明型）、默认后端零污染。
package inference

import (
	"context"
	"testing"
)

func TestInstallHintsInStatus(t *testing.T) {
	sts := NewRegistry().List(context.Background(), false)
	byName := map[string]Status{}
	for _, s := range sts {
		byName[s.Name] = s
	}
	cc, ok := byName["claude-code"]
	if !ok {
		t.Fatal("claude-code 应在注册表")
	}
	if cc.InstallCmd == "" || cc.InstallDocs == "" || cc.InstallNote != "" {
		t.Fatalf("claude-code 应带安装命令与官方文档、无附加说明: %+v", cc)
	}
	dsh := byName["deepseek-harness"]
	if dsh.InstallNote == "" || dsh.InstallCmd != "" || dsh.InstallDocs != "" {
		t.Fatalf("deepseek-harness 应仅说明性引导（无公开安装命令不臆造）: %+v", dsh)
	}
	def := byName[DefaultBackend]
	if def.InstallCmd != "" || def.InstallDocs != "" || def.InstallNote != "" {
		t.Fatalf("eino-adk 默认后端不应带安装引导: %+v", def)
	}
}
