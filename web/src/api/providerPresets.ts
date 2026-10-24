/**
 * REQ-105（M12）：国内常见模型供应商访问配置预设（前端静态清单）。
 * 需求说明允许「前端静态 / 后端种子」二选一实现——取前端静态：清单纯数据驱动，
 * 增删厂商只改本数组，不动交互代码（REQ-77「候选清单开放性」同思路）。
 *
 * 每条预填：协议 + Base URL + 建议首个模型名（REQ-106 自动填充，
 * 用户仅需补 API Key；Key 归属提供商，模型可经 REQ-48 自动发现批量拉取）。
 * REQ-172：新增 protocol 字段与 Anthropic 系预设——Anthropic 官方及 DeepSeek/智谱 GLM/Kimi
 * 的 Anthropic 兼容端点（Base URL 为网关根地址，平台自动拼 /v1/messages）。
 * 接入点与模型名为常见文档口径，厂商侧随版本演进可能调整——预设仅是快捷起点，表单内可改。
 */

export type ProviderProtocol = 'openai_compat' | 'anthropic'

export interface ProviderPreset {
  key: string
  /** 提供商展示名（作为连接名前缀 `{提供商}·{模型}`） */
  name: string
  baseUrl: string
  /** 建议首个模型名 */
  defaultModel: string
  /** 该厂商主打连接类型（个别厂商两条预设分别给 chat / embedding） */
  connType: 'chat' | 'embedding'
  /** 接入协议；缺省 openai_compat（REQ-172 起可选 anthropic） */
  protocol?: ProviderProtocol
  /** 控制台地址（提示去申请 API Key） */
  console?: string
}

export const PROVIDER_PRESETS: ProviderPreset[] = [
  // REQ-408/M204：本地模型预设（零 API key 跑通——Ollama/LM Studio/vLLM OpenAI 兼容端点）。
  // 本地端点无控制台（console 缺省）；defaultModel 为各运行时常见缺省，表单内可改。
  {
    key: 'ollama',
    name: 'Ollama（本机）',
    baseUrl: 'http://127.0.0.1:11434/v1',
    defaultModel: 'qwen2.5:7b',
    connType: 'chat',
  },
  {
    key: 'lmstudio',
    name: 'LM Studio（本机）',
    baseUrl: 'http://127.0.0.1:1234/v1',
    defaultModel: 'local-model',
    connType: 'chat',
  },
  {
    key: 'vllm',
    name: 'vLLM（自建）',
    baseUrl: 'http://127.0.0.1:8000/v1',
    defaultModel: 'Qwen/Qwen2.5-7B-Instruct',
    connType: 'chat',
  },
  {
    key: 'deepseek',
    name: 'DeepSeek 深度求索',
    baseUrl: 'https://api.deepseek.com/v1',
    defaultModel: 'deepseek-chat',
    connType: 'chat',
    console: 'https://platform.deepseek.com',
  },
  {
    key: 'anthropic',
    name: 'Anthropic 官方',
    baseUrl: 'https://api.anthropic.com',
    defaultModel: 'claude-sonnet-4-5',
    connType: 'chat',
    protocol: 'anthropic',
    console: 'https://console.anthropic.com/settings/keys',
  },
  {
    key: 'deepseek-anthropic',
    name: 'DeepSeek（Anthropic 兼容）',
    baseUrl: 'https://api.deepseek.com/anthropic',
    defaultModel: 'deepseek-chat',
    connType: 'chat',
    protocol: 'anthropic',
    console: 'https://platform.deepseek.com',
  },
  {
    key: 'zhipu-anthropic',
    name: '智谱 GLM（Anthropic 兼容）',
    baseUrl: 'https://open.bigmodel.cn/api/anthropic',
    defaultModel: 'glm-4.6',
    connType: 'chat',
    protocol: 'anthropic',
    console: 'https://open.bigmodel.cn',
  },
  {
    key: 'kimi-anthropic',
    name: '月之暗面 Kimi（Anthropic 兼容）',
    baseUrl: 'https://api.moonshot.cn/anthropic',
    defaultModel: 'kimi-k2-turbo-preview',
    connType: 'chat',
    protocol: 'anthropic',
    console: 'https://platform.moonshot.cn',
  },
  {
    key: 'bailian',
    name: '阿里百炼 DashScope',
    baseUrl: 'https://dashscope.aliyuncs.com/compatible-mode/v1',
    defaultModel: 'qwen-plus',
    connType: 'chat',
    console: 'https://bailian.console.aliyun.com',
  },
  {
    key: 'qianfan',
    name: '百度千帆',
    baseUrl: 'https://qianfan.baidubce.com/v2',
    defaultModel: 'ernie-4.5-turbo-128k',
    connType: 'chat',
    console: 'https://console.bce.baidu.com/qianfan',
  },
  {
    key: 'zhipu',
    name: '智谱 GLM',
    baseUrl: 'https://open.bigmodel.cn/api/paas/v4',
    defaultModel: 'glm-4.5',
    connType: 'chat',
    console: 'https://open.bigmodel.cn',
  },
  {
    key: 'kimi',
    name: '月之暗面 Kimi',
    baseUrl: 'https://api.moonshot.cn/v1',
    defaultModel: 'kimi-k2-0905-preview',
    connType: 'chat',
    console: 'https://platform.moonshot.cn',
  },
  {
    key: 'siliconflow',
    name: '硅基流动 SiliconFlow',
    baseUrl: 'https://api.siliconflow.cn/v1',
    defaultModel: 'deepseek-ai/DeepSeek-V3',
    connType: 'chat',
    console: 'https://cloud.siliconflow.cn',
  },
  {
    key: 'minimax',
    name: 'MiniMax',
    baseUrl: 'https://api.minimaxi.com/v1',
    defaultModel: 'MiniMax-M2',
    connType: 'chat',
    console: 'https://platform.minimaxi.com',
  },
  {
    key: 'spark',
    name: '讯飞星火 Spark',
    baseUrl: 'https://spark-api-open.xf-yun.com/v1',
    defaultModel: '4.0Ultra',
    connType: 'chat',
    console: 'https://console.xfyun.cn',
  },
  {
    key: 'qianfan-embedding',
    name: '百度千帆（向量）',
    baseUrl: 'https://qianfan.baidubce.com/v2',
    defaultModel: 'embeddings-v1',
    connType: 'embedding',
    console: 'https://console.bce.baidu.com/qianfan',
  },
]
