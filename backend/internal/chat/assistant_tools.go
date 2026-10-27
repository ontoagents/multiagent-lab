// REQ-186 阶段二：平台助手 L0 只读工具面（分级白名单——只读全开，写类不开放）。
// 注册进通用工具注册表（builtin 来源），由内置助手行（is_builtin=1）的 tools 字段引用；
// 用户 Agent 勾选同一工具亦可用（注册表开放性，REQ-77），风险面为纯只读。
package chat

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"

	einotool "github.com/cloudwego/eino/components/tool"
	"github.com/cloudwego/eino/components/tool/utils"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/fsutil"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/tool"
)

// AssistantDeps L0 工具的只读依赖（main.go 装配时注入；与 api 层 docRead 同源口径）。
type AssistantDeps struct {
	Store         *store.Store
	DocsRoot      string
	KnowledgeRoot string
	// DefaultPrompt REQ-213：内置提示词基座（api.AssistantDefaultPrompt 注入）——
	// get_assistant_config 以「行内 instruction == 基座」判「未微调」（assistant_config 表退役后单源读行）。
	DefaultPrompt string
}

// RegisterAssistantTools 注册 L0 只读工具：doc_read / doc_list / doc_grep / list_model_connections /
// list_agents / list_kbs / get_assistant_config（doc_list/doc_grep 为 REQ-451 平台知识阅读补全件）。
// 同名覆盖幂等。
func RegisterAssistantTools(r *tool.Registry, deps AssistantDeps) error {
	entries := []*tool.Entry{
		assistantDocRead(deps),
		assistantDocList(deps),
		assistantDocGrep(deps),
		assistantListModels(deps),
		assistantListAgents(deps),
		assistantListKBs(deps),
		assistantGetConfig(deps),
	}
	for _, e := range entries {
		if err := r.Register(e); err != nil {
			return err
		}
	}
	return nil
}

// ToolGroupPlatformKnowledge REQ-451：平台知识阅读工具组（管理 UI 分组标注 + /api/tools 透出）。
const ToolGroupPlatformKnowledge = "平台知识"

// resolveDocPath 白名单路径解析（doc_read/doc_list/doc_grep 共用）：仅 docs/ 与
// platform-knowledge/ 两根（research/ 已退役，REQ-184），SafeJoin 防越界。
func resolveDocPath(deps AssistantDeps, rel string) (string, error) {
	clean := filepath.ToSlash(filepath.Clean(rel))
	if strings.Contains(clean, "..") {
		return "", fmt.Errorf("路径越界")
	}
	var root, sub string
	switch {
	case strings.HasPrefix(clean, "research/"):
		return "", fmt.Errorf("research/ 目录已退役（G-5 2026-09-27 并入 platform-knowledge/，REQ-184 收尾 2026-10-22 删除）——请改用 platform-knowledge/ 下的对应文档")
	case strings.HasPrefix(clean, "docs/"):
		root, sub = deps.DocsRoot, strings.TrimPrefix(clean, "docs/")
	case strings.HasPrefix(clean, "platform-knowledge/"):
		root, sub = deps.KnowledgeRoot, strings.TrimPrefix(clean, "platform-knowledge/")
	default:
		return "", fmt.Errorf("仅支持 docs/ 或 platform-knowledge/ 目录下的文档")
	}
	return fsutil.SafeJoin(root, sub)
}

// assistantDocRead L0：读白名单目录（docs/ + platform-knowledge/）下的 .md。
// REQ-451 分段语义：默认全文（48KB 截断并提示总行数）；传 offset_line（1-based）则按行分页
// （max_lines 缺省 400，单次仍受 48KB 上限）——大档（docs/01/02/03 等）此前后半不可达。
// 与 api.docRead 同一白名单与防越界口径（REQ-140/161 语义，供助手精读引用）。
// REQ-184 收尾（2026-10-22）：research/ 分支退役（G-5 并入 platform-knowledge，目录删除）——
// 旧路径返回诚实指引（模型可自纠），不静默 404。
func assistantDocRead(deps AssistantDeps) *tool.Entry {
	return &tool.Entry{
		ID:          "doc_read",
		Name:        "doc_read",
		Description: "读取平台内部文档（docs/、platform-knowledge/ 下 .md，只读）——回答平台使用问题前先精读相关文档；长文档可传 offset_line 分段续读",
		Source:      tool.SourceBuiltin,
		Group:       ToolGroupPlatformKnowledge,
		New: func(ctx context.Context) (einotool.BaseTool, error) {
			fn := func(ctx context.Context, in assistantDocReadIn) (string, error) {
				rel := strings.TrimSpace(in.Path)
				if rel == "" || !strings.HasSuffix(rel, ".md") {
					return "", fmt.Errorf("path 必填且仅支持 .md 文档")
				}
				abs, err := resolveDocPath(deps, rel)
				if err != nil {
					return "", err
				}
				b, err := os.ReadFile(abs)
				if err != nil {
					return "", fmt.Errorf("文档不存在或不可读: %v", err)
				}
				const maxBytes = 48 * 1024
				content := string(b)
				lines := strings.Split(content, "\n")
				if n := len(lines); n > 0 && lines[n-1] == "" { // 文件尾换行不计一行
					lines = lines[:n-1]
				}
				total := len(lines)
				if in.OffsetLine <= 0 {
					if len(content) > maxBytes {
						return content[:maxBytes] + fmt.Sprintf("\n\n…（文档过长已截断，共 %d 行——传 offset_line=%d 分段续读）", total, nextPage(content, maxBytes, 0)), nil
					}
					return content, nil
				}
				// 分段读：[offset_line, offset_line+max_lines)
				maxLines := in.MaxLines
				if maxLines <= 0 {
					maxLines = docReadDefaultPageLines
				}
				if maxLines > docReadDefaultPageLines*4 {
					maxLines = docReadDefaultPageLines * 4
				}
				start := in.OffsetLine - 1
				if start < 0 {
					start = 0
				}
				if start >= len(lines) {
					return "", fmt.Errorf("offset_line=%d 超出文档行数（共 %d 行）", in.OffsetLine, total)
				}
				end := start + maxLines
				if end > len(lines) {
					end = len(lines)
				}
				page := strings.Join(lines[start:end], "\n")
				if len(page) > maxBytes {
					page = trimLastRune(page, maxBytes)
				}
				header := fmt.Sprintf("（第 %d~%d 行 / 共 %d 行）\n", start+1, end, total)
				if end < len(lines) {
					header = fmt.Sprintf("（第 %d~%d 行 / 共 %d 行——继续读传 offset_line=%d）\n", start+1, end, total, end+1)
				}
				return header + page, nil
			}
			return utils.InferTool("doc_read", "读取平台内部文档（docs/、platform-knowledge/ 下 .md）用于精读后回答平台使用问题。入参 path 如 platform-knowledge/02_智能体/智能体模块导读.md；长文档传 offset_line（1-based 起始行）分段续读，max_lines 控制每次行数（默认 400）", fn)
		},
	}
}

const docReadDefaultPageLines = 400

type assistantDocReadIn struct {
	Path       string `json:"path" jsonschema:"文档相对路径（docs/ 或 platform-knowledge/ 前缀，.md 结尾）"`
	OffsetLine int    `json:"offset_line,omitempty" jsonschema:"可选，1-based 起始行——长文档分段续读；缺省读全文（超 48KB 截断）"`
	MaxLines   int    `json:"max_lines,omitempty" jsonschema:"可选，分段读时每次最多行数（默认 400，上限 1600）"`
}

// nextPage 按字节截断位置反推下一页起始行（全文截断提示用）。
func nextPage(content string, maxBytes, startLine int) int {
	// 48KB 截断点所处行号（粗略：截断前内容行数 + offset）
	cut := content
	if len(cut) > maxBytes {
		cut = cut[:maxBytes]
	}
	return strings.Count(cut, "\n") + startLine + 1
}

// trimLastRune 按 UTF-8 边界截断到 maxBytes 内。
func trimLastRune(s string, maxBytes int) string {
	if len(s) <= maxBytes {
		return s
	}
	cut := s[:maxBytes]
	for len(cut) > 0 && !utf8.RuneStart(cut[len(cut)-1]) {
		cut = cut[:len(cut)-1]
	}
	// 末字节是多字节序列起始但序列被截断——回退该 rune
	if r, size := utf8.DecodeLastRuneInString(cut); r == utf8.RuneError && size <= 1 {
		cut = cut[:len(cut)-1]
	}
	return cut
}

// ---------------------------------------------------------------------------
// REQ-451 平台知识阅读补全：doc_list（目录发现）/ doc_grep（词法定位）。
// 治两个缺口——目录树此前只在基座 prompt 硬编码（漂移）；「知道关键词不知道路径」在
// embedding 未配置时无解（search_platform_kb 降级链断裂）。路径口径与 doc_read 一致
// （docs/ + platform-knowledge/ 前缀），命中可直接喂 doc_read。
// ---------------------------------------------------------------------------

// docWalkAll 遍历白名单两根，返回虚拟相对路径（docs/…、platform-knowledge/…）升序清单（.md）。
func docWalkAll(deps AssistantDeps) ([]string, error) {
	var out []string
	pairs := []struct{ prefix, root string }{
		{"docs/", deps.DocsRoot},
		{"platform-knowledge/", deps.KnowledgeRoot},
	}
	for _, p := range pairs {
		werr := filepath.WalkDir(p.root, func(path string, d os.DirEntry, err error) error {
			if err != nil || d == nil || d.IsDir() {
				return nil
			}
			if !strings.HasSuffix(strings.ToLower(path), ".md") {
				return nil
			}
			rel, rerr := filepath.Rel(p.root, path)
			if rerr != nil {
				return nil
			}
			out = append(out, p.prefix+filepath.ToSlash(rel))
			return nil
		})
		if werr != nil && !os.IsNotExist(werr) {
			return nil, werr
		}
	}
	sort.Strings(out)
	return out, nil
}

// docFrontmatter 摘取 md 头部 frontmatter 的 topic/desc 单行值（轻量行解析，不引 yaml 库）。
func docFrontmatter(abs string) (topic, desc string) {
	f, err := os.Open(abs)
	if err != nil {
		return
	}
	defer f.Close()
	buf := make([]byte, 4096)
	n, _ := f.Read(buf)
	head := string(buf[:n])
	if !strings.HasPrefix(head, "---\n") {
		return
	}
	block := head[4:]
	if i := strings.Index(block, "\n---"); i >= 0 {
		block = block[:i]
	}
	for _, line := range strings.Split(block, "\n") {
		line = strings.TrimSpace(line)
		if v, ok := strings.CutPrefix(line, "topic:"); ok && topic == "" {
			topic = strings.Trim(strings.TrimSpace(v), `"'`)
		} else if v, ok := strings.CutPrefix(line, "desc:"); ok && desc == "" {
			desc = strings.Trim(strings.TrimSpace(v), `"'`)
		}
	}
	return
}

// assistantDocList L0：平台文档目录树（路径 + frontmatter topic/desc 摘要），可选 prefix 过滤。
func assistantDocList(deps AssistantDeps) *tool.Entry {
	type in struct {
		Prefix string `json:"prefix,omitempty" jsonschema:"可选，按路径前缀过滤（如 platform-knowledge/04_本体）"`
	}
	newFn := func(ctx context.Context) (einotool.BaseTool, error) {
		fn := func(ctx context.Context, args in) (string, error) {
			prefix := strings.TrimSpace(args.Prefix)
			paths, err := docWalkAll(deps)
			if err != nil {
				return "", err
			}
			var b strings.Builder
			n := 0
			for _, p := range paths {
				if prefix != "" && !strings.HasPrefix(p, prefix) {
					continue
				}
				n++
				abs, aerr := resolveDocPath(deps, p)
				if aerr != nil {
					continue
				}
				topic, desc := docFrontmatter(abs)
				meta := ""
				switch {
				case topic != "" && desc != "":
					meta = " — " + topic + "｜" + desc
				case topic != "":
					meta = " — " + topic
				case desc != "":
					meta = " — " + desc
				}
				fmt.Fprintf(&b, "- %s%s\n", p, meta)
				if b.Len() > 24*1024 { // 目录清单防爆：约千行级截断，附过滤指引
					b.WriteString(fmt.Sprintf("…（清单过长已截断，已列 %d 篇——传 prefix 缩小范围，如 platform-knowledge/04_本体）", n))
					return b.String(), nil
				}
			}
			if n == 0 {
				if prefix != "" {
					return "无匹配文档（检查 prefix 拼写，可用前缀：docs/ 或 platform-knowledge/）", nil
				}
				return "暂无平台文档", nil
			}
			return fmt.Sprintf("平台文档共 %d 篇（可用 doc_read 精读任意路径）：\n%s", n, b.String()), nil
		}
		return utils.InferTool("doc_list", "列出平台内部文档目录树（docs/、platform-knowledge/ 下 .md，含每篇主题摘要；只读）——不知道该读哪篇时先用它发现，可选 prefix 过滤子目录", fn)
	}
	return &tool.Entry{
		ID:          "doc_list",
		Name:        "doc_list",
		Description: "列出平台内部文档目录树（docs/、platform-knowledge/ 下 .md，含主题摘要）——目录发现的正源，替代基座提示词静态目录",
		Source:      tool.SourceBuiltin,
		Group:       ToolGroupPlatformKnowledge,
		New:         newFn,
	}
}

// assistantDocGrep L0：平台文档内容词法检索（正则/字面量，正则非法自动降级字面量）。
func assistantDocGrep(deps AssistantDeps) *tool.Entry {
	const (
		maxFiles = 500
		maxHits  = 50
		maxFile  = 1 << 20
	)
	type in struct {
		Pattern string `json:"pattern" jsonschema:"required"`
		Prefix  string `json:"prefix,omitempty" jsonschema:"可选，限定目录前缀（如 platform-knowledge/04_本体）"`
	}
	newFn := func(ctx context.Context) (einotool.BaseTool, error) {
		fn := func(ctx context.Context, args in) (string, error) {
			pat := strings.TrimSpace(args.Pattern)
			if pat == "" {
				return "", fmt.Errorf("pattern 必填")
			}
			re, err := regexp.Compile(pat)
			if err != nil { // 正则非法降级为字面量包含（诚实容错，沿 tool/grep 先例）
				re = regexp.MustCompile(regexp.QuoteMeta(pat))
			}
			prefix := strings.TrimSpace(args.Prefix)
			paths, err := docWalkAll(deps)
			if err != nil {
				return "", err
			}
			var hits []string
			truncated := false
			files := 0
			for _, p := range paths {
				if prefix != "" && !strings.HasPrefix(p, prefix) {
					continue
				}
				abs, aerr := resolveDocPath(deps, p)
				if aerr != nil {
					continue
				}
				info, serr := os.Stat(abs)
				if serr != nil || info.Size() > maxFile {
					continue
				}
				files++
				if files > maxFiles {
					truncated = true
					break
				}
				b, rerr := os.ReadFile(abs)
				if rerr != nil {
					continue
				}
				for i, line := range strings.Split(string(b), "\n") {
					if re.MatchString(line) {
						if len(hits) >= maxHits {
							truncated = true
							break
						}
						hits = append(hits, fmt.Sprintf("%s:%d:%s", p, i+1, strings.TrimSpace(line)))
					}
				}
				if len(hits) >= maxHits {
					break
				}
			}
			if len(hits) == 0 {
				return "无命中——可换关键词，或用 doc_list 浏览目录", nil
			}
			out := fmt.Sprintf("命中 %d 条（file:line:text，路径可直接喂 doc_read 精读）：\n%s", len(hits), strings.Join(hits, "\n"))
			if truncated {
				out += "\n…（结果已截断——可换更精确的关键词或传 prefix 缩小范围）"
			}
			return out, nil
		}
		return utils.InferTool("doc_grep", "在平台内部文档（docs/、platform-knowledge/ 下 .md）内容中按关键词/正则检索，返回 file:line:text 命中——知道关键词但不知道在哪篇时用；向量检索（search_platform_kb）不可用时的词法兜底", fn)
	}
	return &tool.Entry{
		ID:          "doc_grep",
		Name:        "doc_grep",
		Description: "平台文档内容词法检索（file:line:text 命中）——embedding 未配置或语义检索无果时的兜底定位",
		Source:      tool.SourceBuiltin,
		Group:       ToolGroupPlatformKnowledge,
		New:         newFn,
	}
}

// assistantListModels L0：模型连接清单（脱敏——只回名称/类型/模型名/启用与默认态，不含 Key/BaseURL）。
func assistantListModels(deps AssistantDeps) *tool.Entry {
	return &tool.Entry{
		ID: "list_model_connections", Name: "list_model_connections",
		Description: "列出平台已配置的模型连接（名称/类型/启用态，不含密钥，只读）", Source: tool.SourceBuiltin,
		New: func(ctx context.Context) (einotool.BaseTool, error) {
			fn := func(ctx context.Context, _ struct{}) (string, error) {
				conns, err := deps.Store.ListConnections()
				if err != nil {
					return "", err
				}
				var b strings.Builder
				b.WriteString(fmt.Sprintf("共 %d 条模型连接：\n", len(conns)))
				for _, c := range conns {
					state := "停用"
					if c.Enabled {
						state = "启用"
					}
					def := ""
					if c.IsDefault {
						def = " [默认]"
					}
					key := "未配Key"
					if c.HasKey {
						key = "已配Key"
					}
					fmt.Fprintf(&b, "- %s · %s · %s · %s%s\n", c.Name, c.ConnType, state, key, def)
				}
				return b.String(), nil
			}
			return utils.InferTool("list_model_connections", "列出平台已配置的模型连接（名称/类型/启用态，不含密钥）", fn)
		},
	}
}

// assistantListAgents L0：用户智能体清单（不含内置助手自身）。
func assistantListAgents(deps AssistantDeps) *tool.Entry {
	return &tool.Entry{
		ID: "list_agents", Name: "list_agents",
		Description: "列出平台中的用户智能体清单（名称与描述，只读）", Source: tool.SourceBuiltin,
		New: func(ctx context.Context) (einotool.BaseTool, error) {
			fn := func(ctx context.Context, _ struct{}) (string, error) {
				agents, err := deps.Store.ListAgents()
				if err != nil {
					return "", err
				}
				var b strings.Builder
				n := 0
				for _, a := range agents {
					if a.IsBuiltin {
						continue
					}
					n++
					fmt.Fprintf(&b, "- %s（%s）\n", a.Name, a.Description)
				}
				if n == 0 {
					return "暂无用户智能体", nil
				}
				return fmt.Sprintf("共 %d 个智能体：\n%s", n, b.String()), nil
			}
			return utils.InferTool("list_agents", "列出平台中的用户智能体清单（名称与描述）", fn)
		},
	}
}

// assistantListKBs L0：知识库清单（名称与模式）。
func assistantListKBs(deps AssistantDeps) *tool.Entry {
	return &tool.Entry{
		ID: "list_kbs", Name: "list_kbs",
		Description: "列出平台中的知识库清单（名称与模式，只读）", Source: tool.SourceBuiltin,
		New: func(ctx context.Context) (einotool.BaseTool, error) {
			fn := func(ctx context.Context, _ struct{}) (string, error) {
				kbs, err := deps.Store.ListKnowledgeBases()
				if err != nil {
					return "", err
				}
				if len(kbs) == 0 {
					return "暂无知识库", nil
				}
				var b strings.Builder
				b.WriteString(fmt.Sprintf("共 %d 个知识库：\n", len(kbs)))
				for _, k := range kbs {
					fmt.Fprintf(&b, "- %s（%s 模式）\n", k.Name, k.Mode)
				}
				return b.String(), nil
			}
			return utils.InferTool("list_kbs", "列出平台中的知识库清单（名称与模式）", fn)
		},
	}
}

// assistantGetConfig L0：平台助手自身配置（模型覆盖/温度/提示词是否微调）。
// REQ-213：读 agent 内置行单源（assistant_config 表已退役，REQ-192 遗留收口——原实现读旧表恒空值）。
func assistantGetConfig(deps AssistantDeps) *tool.Entry {
	return &tool.Entry{
		ID: "get_assistant_config", Name: "get_assistant_config",
		Description: "查询平台助手自身的配置（模型覆盖/温度/提示词是否微调，只读）", Source: tool.SourceBuiltin,
		New: func(ctx context.Context) (einotool.BaseTool, error) {
			fn := func(ctx context.Context, _ struct{}) (string, error) {
				a, err := deps.Store.GetAgent("builtin-assistant")
				if err != nil {
					return "", err
				}
				tuned := strings.TrimSpace(a.Instruction) != "" && a.Instruction != deps.DefaultPrompt
				out := map[string]any{
					"model_conn_id": derefAgentStr(a.ModelConnID),
					"temperature":   a.Temperature,
					"prompt_tuned":  tuned,
				}
				b, _ := json.MarshalIndent(out, "", "  ")
				return string(b), nil
			}
			return utils.InferTool("get_assistant_config", "查询平台助手自身的配置（模型覆盖/温度/提示词是否微调）", fn)
		},
	}
}

// derefAgentStr nil 安全解引用（chat 包本地版，避免引 api）。
func derefAgentStr(p *string) string {
	if p == nil {
		return ""
	}
	return *p
}
