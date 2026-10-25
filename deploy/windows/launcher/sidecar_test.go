package main

import (
	"path/filepath"
	"testing"
)

// TestProbeSidecarPython 机器容差断言：命中时必须是存在的绝对路径（LookPath 语义，ToSlash
// 形态合法）；未命中（机器无 python/rdflib）返回空串不 panic。SIDECAR_PYTHON 已设时尊重（恒空串）。
func TestProbeSidecarPython(t *testing.T) {
	t.Setenv("SIDECAR_PYTHON", `C:\some\explicit\python.exe`)
	if got := probeSidecarPython(); got != "" {
		t.Fatalf("已设 SIDECAR_PYTHON 应尊重不探测，got %q", got)
	}
	t.Setenv("SIDECAR_PYTHON", "")
	if got := probeSidecarPython(); got != "" {
		if !filepath.IsAbs(got) {
			t.Fatalf("命中应为绝对路径，got %q", got)
		}
	}
}
