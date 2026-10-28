package chat

// REQ-456/M252：上下文准入链 Retriever 接口化（eino 能力深化批⑤）。
// recallChain（REQ-201 A4 Provider 链）形式化为 eino components/retriever.Retriever 接口投影——
// 双向适配，零行为变更（沿 REQ-355 chat.KBSource 反转先例）：
//
//  1. providerRetriever：把平台 RecallProvider 投影为 eino retriever.Retriever——
//     外部消费方可经统一 Retrieve 语义调用平台召回链（KB/伴生两源）。
//  2. retrieverProvider：把任意 eino retriever.Retriever（含自定义外部检索源）适配为
//     RecallProvider 注册进 recallChain 尾部——外部检索源即插即用（详设验收点）；
//     命中文档作为 System 消息注入（口径与 recallKB 同型：失败降级不阻断+retrieval 事件留痕）。
//
// 语义边界：Retriever.Retrieve 是「无注入副作用」的纯检索签名，平台 Provider 带注入与事件
// 透明——投影侧把注入结果折叠回 []Document（仅新增消息），注入副作用照常发生（文档化）。

import (
	"context"
	"fmt"
	"sort"

	"github.com/cloudwego/eino/components/retriever"
	"github.com/cloudwego/eino/schema"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

// 编译期接口断言。
var (
	_ retriever.Retriever = (*providerRetriever)(nil)
)

// providerRetriever 平台 RecallProvider → eino retriever.Retriever 投影（REQ-456①）。
type providerRetriever struct {
	s       *Service
	source  string // 召回源名（context.stats 的 source 口径；空=整链投影）
	conv    *store.Conversation
	agent   *store.Agent
	runID   string
	histMsgs []*schema.Message
	emit    EmitFn
}

// NewProviderRetriever 把平台上下文准入链（或单源）投影为 eino Retriever。
// histMsgs 为基线历史——Retrieve 返回「召回新增的消息」折叠为 Document（Content=文本，
// MetaData.source=源名）。注入副作用照常发生（Provider 语义；此处基线副本不回传调用方）。
func (s *Service) NewProviderRetriever(source string, conv *store.Conversation, agent *store.Agent, runID string, histMsgs []*schema.Message, emit EmitFn) retriever.Retriever {
	return &providerRetriever{s: s, source: source, conv: conv, agent: agent, runID: runID, histMsgs: histMsgs, emit: emit}
}

// Retrieve 执行召回链并折叠新增消息为 []Document。
func (r *providerRetriever) Retrieve(ctx context.Context, query string, _ ...retriever.Option) ([]*schema.Document, error) {
	chain := r.s.recallChain()
	if r.source != "" {
		chain = nil
		for _, p := range namedRecallChain(r.s) {
			if p.name == r.source {
				chain = []RecallProvider{p.provider}
				break
			}
		}
		if chain == nil {
			return nil, fmt.Errorf("未知召回源 %q（可用：kb/companion）", r.source)
		}
	}
	base := append([]*schema.Message{}, r.histMsgs...)
	hist := base
	for _, rp := range chain {
		hist = rp(ctx, r.conv, r.agent, r.runID, query, hist, r.emit)
	}
	out := make([]*schema.Document, 0, len(hist)-len(base))
	for i := len(base); i < len(hist); i++ {
		m := hist[i]
		out = append(out, &schema.Document{Content: m.Content, MetaData: map[string]any{"role": string(m.Role), "source": "recall_projection"}})
	}
	return out, nil
}

type namedProvider struct {
	name     string
	provider RecallProvider
}

// namedRecallChain 带源名的准入链（与 recallChain 同序；REQ-456 投影/反向注册共用）。
func namedRecallChain(s *Service) []namedProvider {
	return []namedProvider{{name: "kb", provider: s.recallKB}, {name: "companion", provider: s.recallCompanion}}
}

// retrieverProvider 任意 eino retriever.Retriever → RecallProvider 适配（REQ-456②反向插入）。
// 命中文档聚合为单条 System 消息注入（口径与 recallKB 同型：失败降级不阻断+retrieval 事件留痕，
// 事件名带 source 后缀以区分外部源）。
func retrieverProvider(source string, r retriever.Retriever) RecallProvider {
	return func(ctx context.Context, conv *store.Conversation, agent *store.Agent, runID, input string, histMsgs []*schema.Message, emit EmitFn) []*schema.Message {
		docs, err := r.Retrieve(ctx, input, retriever.WithTopK(defaultRecallTopK))
		if err != nil {
			// 失败降级不阻断（准入链纪律）；run.warning 如实留痕
			if emit != nil {
				emit(newEvent("run.warning", runID, map[string]any{
					"message": fmt.Sprintf("外部检索源 %s 召回失败，本轮跳过：%v", source, err),
				}))
			}
			return histMsgs
		}
		if len(docs) == 0 {
			return histMsgs
		}
		content := ""
		for i, d := range docs {
			if i > 0 {
				content += "\n\n"
			}
			content += d.Content
		}
		if emit != nil {
			emit(newEvent("retrieval", runID, map[string]any{
				"source": source, "query_chars": len(input), "hits": len(docs), "chars": len(content),
			}))
		}
		return append(histMsgs, schema.SystemMessage("【外部检索：" + source + "】\n" + content))
	}
}

// defaultRecallTopK 外部检索源默认条数（与 KB 召回 topK 默认同档）。
const defaultRecallTopK = 5

// RegisterRetrieverSource 把外部 eino Retriever 注册进上下文准入链尾部（REQ-456 验收点：
// 自定义外部检索源可插入）。运行期注册作用于后续所有 Run/Resume；重复 source 覆盖。
func (s *Service) RegisterRetrieverSource(source string, r retriever.Retriever) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.externalRetrievers == nil {
		s.externalRetrievers = map[string]retriever.Retriever{}
	}
	s.externalRetrievers[source] = r
}

// recallChainWithExternal 完整准入链：内置两源 + 外部注册源（map 遍历序不定——按 source 名稳定序）。
func (s *Service) recallChainWithExternal() []RecallProvider {
	chain := append([]RecallProvider{}, s.recallChain()...)
	s.mu.Lock()
	defer s.mu.Unlock()
	for _, n := range externalSourceNamesLocked(s) {
		chain = append(chain, retrieverProvider(n, s.externalRetrievers[n]))
	}
	return chain
}

// externalSourceNames 外部检索源名清单（按名稳定序；自身加锁——外部消费方用）。
func externalSourceNames(s *Service) []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return externalSourceNamesLocked(s)
}

// externalSourceNamesLocked 无锁变体（调用方须持 s.mu——recallChainWithExternal 组合读用，
// 防非重入死锁）。
func externalSourceNamesLocked(s *Service) []string {
	names := make([]string, 0, len(s.externalRetrievers))
	for k := range s.externalRetrievers {
		names = append(names, k)
	}
	sort.Strings(names)
	return names
}
