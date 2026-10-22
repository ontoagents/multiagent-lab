package skill

// REQ-371/M169：目录式技能包导入解析单测——frontmatter 变体、多技能 zip、
// 包内重名、缺正文、资源限额与二进制跳过、无 SKILL.md 守卫。

import (
	"archive/zip"
	"bytes"
	"strconv"
	"strings"
	"testing"
)

func buildZip(t *testing.T, files map[string]string) (*bytes.Reader, int64) {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	b := buf.Bytes()
	return bytes.NewReader(b), int64(len(b))
}

const mdA = `---
name: 技能甲
description: "测试技能 A 的描述"
license: MIT
metadata:
  version: "1.0"
---

# 技能甲正文

按步骤执行……
`

func TestParseZipMultiSkill(t *testing.T) {
	r, size := buildZip(t, map[string]string{
		"alpha/SKILL.md":          mdA,
		"alpha/scripts/run.py":    "print('hi')\n",
		"alpha/scripts/README.md": "# alpha 脚本\n",
		"beta/SKILL.md":           "---\nname: 技能乙\ndescription: B\n---\n\nB 正文\n",
	})
	imp, err := ParseSkillZip(r, size)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(imp.Skills) != 2 || len(imp.Failed) != 0 || len(imp.Skipped) != 0 {
		t.Fatalf("应 2 技能 0 失败 0 跳过: %+v", imp)
	}
	// 确定性排序：包内路径字典序 alpha < beta
	if imp.Skills[0].Name != "技能甲" || imp.Skills[1].Name != "技能乙" {
		t.Fatalf("技能名不符: %s / %s", imp.Skills[0].Name, imp.Skills[1].Name)
	}
	a := imp.Skills[0]
	if !strings.Contains(a.Instruction, "技能甲正文") {
		t.Fatalf("正文未取到: %q", a.Instruction)
	}
	if a.Description != "测试技能 A 的描述" {
		t.Fatalf("引号描述未去引号: %q", a.Description)
	}
	if len(a.Resources) != 2 {
		t.Fatalf("应有 2 个文本资源: %+v", a.Resources)
	}
	if a.Resources[0].Name != "scripts/README.md" || a.Resources[1].Name != "scripts/run.py" {
		t.Fatalf("资源相对路径不符: %+v", a.Resources)
	}
}

func TestParseZipNameFallbackAndDuplicate(t *testing.T) {
	r, size := buildZip(t, map[string]string{
		"x/SKILL.md": "无 frontmatter，直接正文\n",
		"y/SKILL.md": "---\nname: 技能甲\n---\n正文甲\n",
		"z/SKILL.md": "---\nname: 技能甲\n---\n正文甲重复\n",
	})
	imp, err := ParseSkillZip(r, size)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(imp.Skills) != 2 || len(imp.Skipped) != 1 {
		t.Fatalf("应 2 技能 1 跳过: %+v", imp)
	}
	if imp.Skipped[0].Reason == "" || !strings.Contains(imp.Skipped[0].Reason, "重名") {
		t.Fatalf("跳过原因应含「重名」: %+v", imp.Skipped)
	}
	// x 无 frontmatter：name 回落目录名 x；description 空（目录渲染由平台兜底）
	if imp.Skills[0].Name != "x" || imp.Skills[0].Description != "" {
		t.Fatalf("无 frontmatter 回落不符: %+v", imp.Skills[0])
	}
}

func TestParseZipEmptyBodyFails(t *testing.T) {
	r, size := buildZip(t, map[string]string{
		"only-fm/SKILL.md": "---\nname: 空正文\ndescription: 没有正文\n---\n\n",
	})
	imp, err := ParseSkillZip(r, size)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(imp.Skills) != 0 || len(imp.Failed) != 1 || !strings.Contains(imp.Failed[0].Reason, "正文为空") {
		t.Fatalf("空正文应判 Failed: %+v", imp)
	}
}

func TestParseZipResourceGuards(t *testing.T) {
	big := strings.Repeat("a", MaxResourceFileBytes+1) // 超单文件上限
	bin := string([]byte{0x00, 0x01, 0xff, 0xfe, 'b'})
	r, size := buildZip(t, map[string]string{
		"s/SKILL.md": "---\nname: s\n---\n正文\n",
		"s/big.txt":  big,
		"s/bin.dat":  bin,
		"s/ok.txt":   "ok\n",
		"loose.txt":  "散落文件不导入\n",
	})
	imp, err := ParseSkillZip(r, size)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	if len(imp.Skills) != 1 {
		t.Fatalf("应 1 技能: %+v", imp)
	}
	s := imp.Skills[0]
	if len(s.Resources) != 1 || s.Resources[0].Name != "ok.txt" {
		t.Fatalf("仅 ok.txt 应入资源: %+v", s.Resources)
	}
	joined := strings.Join(s.Notes, "\n")
	if !strings.Contains(joined, "超过单文件") || !strings.Contains(joined, "二进制") {
		t.Fatalf("超限与二进制应诚实标注: %v", s.Notes)
	}
}

func TestParseZipManyResourcesCap(t *testing.T) {
	files := map[string]string{"s/SKILL.md": "---\nname: s\n---\n正文\n"}
	for i := 0; i < MaxResourcesPerSkill+5; i++ {
		files["s/f"+strconv.Itoa(i)+".txt"] = "c\n"
	}
	r, size := buildZip(t, files)
	imp, err := ParseSkillZip(r, size)
	if err != nil {
		t.Fatalf("解析失败: %v", err)
	}
	s := imp.Skills[0]
	if len(s.Resources) != MaxResourcesPerSkill {
		t.Fatalf("资源数应封顶 %d: %d", MaxResourcesPerSkill, len(s.Resources))
	}
	if len(s.Notes) == 0 || !strings.Contains(s.Notes[0], "资源数超过") {
		t.Fatalf("超限应标注: %v", s.Notes)
	}
}

func TestParseZipGuards(t *testing.T) {
	// 无 SKILL.md
	r, size := buildZip(t, map[string]string{"a/readme.md": "x"})
	if _, err := ParseSkillZip(r, size); err == nil {
		t.Fatal("无 SKILL.md 应报错")
	}
	// 非 zip 内容
	if _, err := ParseSkillZip(bytes.NewReader([]byte("not a zip")), 9); err == nil {
		t.Fatal("非 zip 应报错")
	}
	// 顶层单文件技能（无目录层级）
	r2, size2 := buildZip(t, map[string]string{"SKILL.md": "---\nname: 顶层\n---\n顶层正文\n"})
	imp, err := ParseSkillZip(r2, size2)
	if err != nil {
		t.Fatalf("顶层 SKILL.md 应可解析: %v", err)
	}
	if len(imp.Skills) != 1 || imp.Skills[0].Name != "顶层" {
		t.Fatalf("顶层技能解析不符: %+v", imp)
	}
}

func TestFrontmatterBlockScalar(t *testing.T) {
	md := "---\ndescription: |\n  第一行\n  第二行\nname: 块标量\n---\n正文\n"
	fields, body := splitFrontmatter(md)
	if fields["name"] != "块标量" {
		t.Fatalf("name 解析不符: %+v", fields)
	}
	if !strings.Contains(fields["description"], "第一行") || !strings.Contains(fields["description"], "第二行") {
		t.Fatalf("块标量 description 应合并: %q", fields["description"])
	}
	if strings.TrimSpace(body) != "正文" {
		t.Fatalf("正文不符: %q", body)
	}
}
