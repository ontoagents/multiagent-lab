// Package skill 内：目录式技能包导入解析（REQ-371/M169 技能模块导入功能）。
// 目标形态=业界 Agent Skills 标准（SKILL.md + frontmatter name/description + 同目录附加资源），
// 与平台 Skill 模型映射：name→Name、description→Description、正文→Instruction（load_skill 懒加载正文）、
// 同目录文本文件→Resources（REQ-120 resources 字段，注入消费走 P2）；
// allowed-tools 等工具面字段不映射平台工具注册表（置空，由使用者按平台工具勾选）。
// 只解析不落库：store 写入与同名跳过语义在 api 层（沿 M23 配置导入「同名跳过不覆盖」）。
package skill

import (
	"archive/zip"
	"bytes"
	"fmt"
	"io"
	"path"
	"sort"
	"strings"
	"unicode/utf8"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

// 导入防线（诚实标注而非静默丢弃：超限记入 Notes）。
const (
	MaxSkillsPerZip      = 500 // 单包技能数上限
	MaxUncompressedBytes = 128 << 20
	MaxInstructionBytes  = 256 << 10 // 正文上限，超限截断
	MaxResourceFileBytes = 64 << 10  // 单资源上限，超限跳过
	MaxResourcesPerSkill = 30        // 单技能资源数上限
	MaxResourceBytes     = 384 << 10 // 单技能资源总量上限
	maxNameRunes         = 80
)

// ParsedSkill 单个技能包的解析结果（未落库）。
type ParsedSkill struct {
	Dir         string                `json:"dir"` // 包内目录（展示用）
	Name        string                `json:"name"`
	Description string                `json:"description,omitempty"`
	Instruction string                `json:"instruction"`
	Resources   []store.SkillResource `json:"resources"`
	Notes       []string              `json:"notes,omitempty"` // 截断/资源超限等诚实标注
}

// SkipItem 跳过/失败条目（name+原因，人话）。
type SkipItem struct {
	Name   string `json:"name"`
	Reason string `json:"reason"`
}

// ZipImport 一次 zip 的解析汇总。
type ZipImport struct {
	Skills  []*ParsedSkill `json:"skills"` // 拟新建（按包内路径排序，确定性）
	Skipped []SkipItem     `json:"skipped,omitempty"`
	Failed  []SkipItem     `json:"failed,omitempty"`
}

// ParseSkillZip 解析 zip 形态的目录式技能包：识别包内全部 SKILL.md（任意深度），
// 每个以其所在目录为一个技能，同目录其余文本文件收为 resources。
func ParseSkillZip(r io.ReaderAt, size int64) (*ZipImport, error) {
	zr, err := zip.NewReader(r, size)
	if err != nil {
		return nil, fmt.Errorf("无法按 zip 解析上传文件（仅支持 .zip 格式的技能包）")
	}
	// 防解压炸弹：全部条目声明尺寸求和即需有界（读取时再按 LimitReader 二次兜底）
	var declared int64
	for _, zf := range zr.File {
		declared += int64(zf.UncompressedSize64)
	}
	if declared > MaxUncompressedBytes {
		return nil, fmt.Errorf("解压后总体积超过 %dMB 上限", MaxUncompressedBytes>>20)
	}

	// 1) 收集 SKILL.md 条目并按目录归组（路径统一 / 分隔，兼容 Windows 打包的 \）
	type group struct {
		root  string // 含尾斜杠；顶层为 ""
		skill *zip.File
		files []*zip.File
	}
	byRoot := map[string]*group{}
	var order []string
	for _, zf := range zr.File {
		if zf.FileInfo().IsDir() {
			continue
		}
		p := strings.ReplaceAll(zf.Name, "\\", "/")
		p = strings.TrimPrefix(p, "./")
		base := path.Base(p)
		if strings.EqualFold(base, "SKILL.md") {
			root := ""
			if dir := path.Dir(p); dir != "." && dir != "/" {
				root = dir + "/"
			}
			if _, ok := byRoot[root]; !ok {
				order = append(order, root)
			}
			byRoot[root] = &group{root: root, skill: zf}
		}
	}
	if len(byRoot) == 0 {
		return nil, fmt.Errorf("包内未找到 SKILL.md（目录式技能包需含 SKILL.md）")
	}
	if len(byRoot) > MaxSkillsPerZip {
		return nil, fmt.Errorf("包内技能数 %d 超过单次导入上限 %d", len(byRoot), MaxSkillsPerZip)
	}
	sort.Strings(order) // 技能按目录路径字典序，输出确定性
	// 2) 其余文件按「最长匹配 root」归入所属技能（支持 scripts/ 等子目录）
	for _, zf := range rangeFiles(zr) {
		p := strings.ReplaceAll(zf.Name, "\\", "/")
		p = strings.TrimPrefix(p, "./")
		if strings.EqualFold(path.Base(p), "SKILL.md") {
			continue
		}
		best := ""
		found := false
		for _, root := range order {
			if strings.HasPrefix(p, root) && len(root) >= len(best) {
				best, found = root, true
			}
		}
		if !found {
			continue // 散落在技能目录外的文件不导入（诚实忽略）
		}
		byRoot[best].files = append(byRoot[best].files, zf)
	}
	// 资源按包内路径排序：输出确定性（同名判断/测试断言稳定）
	for _, g := range byRoot {
		sort.Slice(g.files, func(i, j int) bool { return g.files[i].Name < g.files[j].Name })
	}

	// 3) 逐技能解析
	imp := &ZipImport{}
	seenName := map[string]bool{}
	for _, root := range order {
		g := byRoot[root]
		md, err := readZipFile(g.skill)
		if err != nil {
			imp.Failed = append(imp.Failed, SkipItem{Name: dirLabel(root), Reason: "SKILL.md 读取失败: " + err.Error()})
			continue
		}
		ps := parseSkillMD(string(md), root)
		if strings.TrimSpace(ps.Instruction) == "" {
			imp.Failed = append(imp.Failed, SkipItem{Name: ps.Name, Reason: "SKILL.md 正文为空，无法作为技能指令导入"})
			continue
		}
		if seenName[ps.Name] {
			imp.Skipped = append(imp.Skipped, SkipItem{Name: ps.Name, Reason: "与同包内前序技能重名，仅导入第一个"})
			continue
		}
		seenName[ps.Name] = true
		imp.Skills = append(imp.Skills, ps)
		// 资源收集（文本、限额，超限诚实标注）
		resTotal := 0
		for _, rf := range g.files {
			if len(ps.Resources) >= MaxResourcesPerSkill {
				ps.Notes = append(ps.Notes, fmt.Sprintf("资源数超过 %d 上限，其余文件未导入", MaxResourcesPerSkill))
				break
			}
			rc, err := readZipFile(rf)
			if err != nil {
				ps.Notes = append(ps.Notes, "资源 "+relName(root, rf)+" 读取失败，未导入")
				continue
			}
			rel := relName(root, rf)
			if len(rc) > MaxResourceFileBytes {
				ps.Notes = append(ps.Notes, fmt.Sprintf("资源 %s 超过单文件 %dKB 上限，未导入", rel, MaxResourceFileBytes>>10))
				continue
			}
			if !utf8.Valid(rc) || bytes.IndexByte(rc, 0) >= 0 {
				ps.Notes = append(ps.Notes, "资源 "+rel+" 为二进制/非 UTF-8 内容，未导入")
				continue
			}
			if resTotal+len(rc) > MaxResourceBytes {
				ps.Notes = append(ps.Notes, fmt.Sprintf("资源总量超过 %dKB 上限，%s 及其余文件未导入", MaxResourceBytes>>10, rel))
				break
			}
			resTotal += len(rc)
			ps.Resources = append(ps.Resources, store.SkillResource{Name: rel, Content: string(rc)})
		}
	}
	return imp, nil
}

// parseSkillMD 解析单个 SKILL.md：frontmatter 取 name/description，正文作 Instruction；
// 无 frontmatter 时 name 回落目录名、description 留空（目录渲染由平台兜底）。
func parseSkillMD(md, root string) *ParsedSkill {
	ps := &ParsedSkill{Dir: strings.TrimSuffix(root, "/"), Resources: []store.SkillResource{}}
	fields, body := splitFrontmatter(md)
	name := sanitizeName(fields["name"])
	if name == "" {
		name = sanitizeName(path.Base(strings.TrimSuffix(root, "/")))
	}
	if name == "" {
		name = "imported-skill"
	}
	ps.Name = name
	ps.Description = strings.TrimSpace(fields["description"])
	ps.Instruction = strings.TrimSpace(body)
	if len(ps.Instruction) > MaxInstructionBytes {
		ps.Instruction = truncateUTF8(ps.Instruction, MaxInstructionBytes) + "\n\n（导入时截断：正文超过 256KB 上限，完整内容见原 SKILL.md）"
		ps.Notes = append(ps.Notes, "正文超过 256KB，已截断")
	}
	return ps
}

// splitFrontmatter 拆分 YAML frontmatter（仅解析顶层单行键值与块标量，其余键忽略）。
func splitFrontmatter(md string) (fields map[string]string, body string) {
	fields = map[string]string{}
	s := strings.TrimLeft(md, "\uFEFF \t\r\n")
	if !strings.HasPrefix(s, "---") {
		return nil, md
	}
	lines := strings.SplitN(s, "\n", -1)
	if strings.TrimSpace(lines[0]) != "---" {
		return nil, md
	}
	end := -1
	for i := 1; i < len(lines); i++ {
		if strings.TrimSpace(lines[i]) == "---" {
			end = i
			break
		}
	}
	if end < 0 {
		return nil, md
	}
	// 顶层键值：`key: value`；value 为 | 或 > 时收集后续缩进行作为块标量
	var blockKey, blockStyle string
	for _, ln := range lines[1:end] {
		indented := len(ln) > 0 && (ln[0] == ' ' || ln[0] == '\t')
		if indented && blockKey != "" {
			fields[blockKey] += strings.TrimSpace(ln)
			if blockStyle == "|" {
				fields[blockKey] += "\n"
			} else {
				fields[blockKey] += " "
			}
			continue
		}
		blockKey = ""
		k, v, ok := strings.Cut(ln, ":")
		if !ok || strings.ContainsAny(k, " \t-") {
			continue
		}
		key, val := strings.TrimSpace(k), strings.TrimSpace(v)
		if val == "|" || val == ">" || val == "|-" || val == ">-" {
			blockKey, blockStyle = key, val
			fields[key] = ""
			continue
		}
		fields[key] = unquote(val)
	}
	body = strings.Join(lines[end+1:], "\n")
	return fields, body
}

func unquote(v string) string {
	if len(v) >= 2 && (v[0] == '"' && v[len(v)-1] == '"' || v[0] == '\'' && v[len(v)-1] == '\'') {
		return v[1 : len(v)-1]
	}
	return v
}

// truncateUTF8 按字节上限截断且不切断多字节字符。
func truncateUTF8(s string, n int) string {
	if len(s) <= n {
		return s
	}
	for n > 0 && !utf8.RuneStart(s[n]) {
		n--
	}
	return s[:n]
}

// sanitizeName 技能名净化：去路径与控制符、压缩空白、限长（平台名唯一约束由 store 承担）。
func sanitizeName(s string) string {
	s = strings.ReplaceAll(s, "/", "-")
	s = strings.ReplaceAll(s, "\\", "-")
	var b strings.Builder
	for _, r := range s {
		if r == 0x7f || r < 0x20 {
			continue
		}
		b.WriteRune(r)
	}
	s = strings.Join(strings.Fields(b.String()), " ")
	runes := []rune(s)
	if len(runes) > maxNameRunes {
		s = string(runes[:maxNameRunes])
	}
	return s
}

func readZipFile(zf *zip.File) ([]byte, error) {
	rc, err := zf.Open()
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(io.LimitReader(rc, MaxUncompressedBytes+1))
}

func relName(root string, zf *zip.File) string {
	p := strings.ReplaceAll(zf.Name, "\\", "/")
	return strings.TrimPrefix(p, root)
}

func dirLabel(root string) string {
	return strings.TrimSuffix(root, "/")
}

func rangeFiles(zr *zip.Reader) []*zip.File {
	out := make([]*zip.File, 0, len(zr.File))
	for _, zf := range zr.File {
		if !zf.FileInfo().IsDir() {
			out = append(out, zf)
		}
	}
	return out
}
