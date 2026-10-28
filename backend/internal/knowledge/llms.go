package knowledge

// REQ-372 P3/M265：知识体系 L3 服务层——llms.txt 聚合视图（58 号 §4.4「L3 服务层之一」/
// §11.3 方向 3 顺手件）。卡为事实源单源：llms.md 是**生成物**（卡 frontmatter → 机器可读聚合），
// 生成而非手维——漂移由常跑测试 TestLlmsTxtInSync 强制（改卡后
// `KS_REGEN=1 go test ./internal/knowledge -run TestLlmsTxtInSync` 重新生成）。
// 消费面：①落库 llms.md 随平台 KB 同步+doc_read 可读（平台助手引用知识体系作答，零新工具）；
// ②GET /api/knowledge/llms.txt 请求时生成（外部 LLM 消费口径，恒新鲜零漂移）。
// GraphRAG 语义检索/知识卡问答深度接入维持触发驱动（§11.3 方向 5，不主动领取）。

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// llmsFileName 知识体系目录下的聚合文件名（.md 形态——平台 KB 同步与 doc_read 均只认 .md）。
const llmsFileName = "llms.md"

// llmsCard 聚合视图中的单卡条目。
type llmsCard struct {
	path, topic, kind, domain, desc string
	prereq, sources                 int
}

// collectLlmsCards 扫描知识体系目录收集全部卡（kind 非空且位于 <线>/cards/ 两级下），按线分组。
// 确定性：线与卡均按路径字典序。
func collectLlmsCards(root, repoRel string) (byLine map[string][]llmsCard, kinds map[string]int, maxSynced string, err error) {
	byLine = map[string][]llmsCard{}
	kinds = map[string]int{}
	werr := filepath.WalkDir(root, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".md") {
			return err
		}
		b, rerr := os.ReadFile(path)
		if rerr != nil {
			return nil // 不可读文件跳过（不阻断聚合）
		}
		meta, perr := ParseCardText(string(b))
		if perr != nil || meta.Kind == "" {
			return nil // 导读/主档/llms.md 等非卡文档不入聚合
		}
		dir := filepath.ToSlash(filepath.Dir(path))
		if filepath.Base(dir) != "cards" {
			return nil // 卡必须位于 <线>/cards/ 两级之下
		}
		line := filepath.Base(filepath.Dir(dir))
		c := llmsCard{path: cardRelID(root, repoRel, path), topic: meta.Topic, kind: meta.Kind,
			domain: meta.Domain, desc: meta.Desc, prereq: len(meta.Prereq), sources: len(meta.Sources)}
		if c.topic == "" {
			c.topic = strings.TrimSuffix(filepath.Base(path), ".md")
		}
		byLine[line] = append(byLine[line], c)
		kinds[meta.Kind]++
		if meta.Synced > maxSynced {
			maxSynced = meta.Synced
		}
		return nil
	})
	if werr != nil {
		return nil, nil, "", werr
	}
	for l := range byLine {
		cs := byLine[l]
		sort.Slice(cs, func(i, j int) bool { return cs[i].path < cs[j].path })
	}
	return byLine, kinds, maxSynced, nil
}

// BuildLlmsDoc 完整 llms.md（frontmatter+正文）。llms.txt 惯例形态：H1+引语+分节清单。
// frontmatter 亦为生成：synced 取卡 synced 最大值（确定性——不用当日，避免逐日漂移）；
// 无 kind 字段故不进图谱投影。同输入同字节（drift 测试依赖）。
func BuildLlmsDoc(root, repoRel string) (string, error) {
	byLine, kinds, maxSynced, err := collectLlmsCards(root, repoRel)
	if err != nil {
		return "", err
	}
	lines := make([]string, 0, len(byLine))
	for l := range byLine {
		lines = append(lines, l)
	}
	sort.Strings(lines)

	var b strings.Builder
	b.WriteString("---\nmodule: 知识体系\ntopic: LLM 可读聚合视图（llms.txt 形态）\n")
	b.WriteString("desc: 全部知识卡的机器可读索引——生成器从卡 frontmatter 聚合（生成而非手维，漂移由常跑测试强制）\n")
	if maxSynced != "" {
		fmt.Fprintf(&b, "synced: %s\n", maxSynced)
	}
	b.WriteString("sources:\n  - platform-knowledge/08_调研预研/58_核心知识体系提取可行方案.md\n  - REQ-372\n---\n\n")
	b.WriteString("# 平台核心知识体系（LLM 聚合视图）\n\n")
	fmt.Fprintf(&b, "> REQ-372 知识工程三层资产：三型知识卡（概念/决策/实践）× 三线学习路径的机器可读聚合。本文件为生成物（勿手改）——卡 frontmatter 为事实源，改卡后 `KS_REGEN=1 go test ./internal/knowledge -run TestLlmsTxtInSync` 重新生成。人读入口 [00_导读.md](%s/09_知识体系/00_导读.md)。\n", repoRel)
	fmt.Fprintf(&b, ">\n> 共 %d 卡：概念 %d · 决策 %d · 实践 %d ｜ 卡 synced 最新 %s\n\n",
		kinds["concept"]+kinds["decision"]+kinds["practice"], kinds["concept"], kinds["decision"], kinds["practice"], maxSynced)
	for _, l := range lines {
		cs := byLine[l]
		fmt.Fprintf(&b, "## %s（%d 卡）\n\n", l, len(cs))
		for _, c := range cs {
			fmt.Fprintf(&b, "- [%s](%s): %s｜%s｜%s｜前置 %d·源指针 %d\n", c.topic, c.path, c.kind, c.domain, c.desc, c.prereq, c.sources)
		}
		b.WriteString("\n")
	}
	return b.String(), nil
}

// RegenerateLlmsDoc 重写知识体系目录下的 llms.md（测试 KS_REGEN=1 与运维重生成共用）。
func RegenerateLlmsDoc(root, repoRel string) error {
	doc, err := BuildLlmsDoc(root, repoRel)
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(root, llmsFileName), []byte(doc), 0o644)
}
