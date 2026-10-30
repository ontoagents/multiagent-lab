package evaldata

// REQ-526/M333（P08）：真实语料基准——platform-knowledge 真实文档节选（README 收录规则段+
// 16 号部署段）+20 题 gold 标注（内容子串，离线词法臂确定性）。
// 区别于种子语料（教学构造）：真实文档结构（标题/列表/代码块/表格）下检索质量才可比；
// 向量臂经 KB_EVAL_DB 形态可选（KB-22 先例），缺省词法臂离线跑。

// RealCorpus 真实语料（两文档拼接节选，~60 行——切分后 3~5 chunks）。
const RealCorpus = `# 平台知识（Platform Knowledge）

> 仓库路径 platform-knowledge/；前端「平台知识」页（REQ-116/161）构建期内联本目录。收录与归属目录驱动（REQ-188 起+数字前缀规范）：目录即页面树——顶层目录 = L1 组、子目录 = L2、文档 = 主题页；组序/组内序均由名称数字前缀决定，新增目录或文档落入即上页并自动入位（重建生效），无需改前端代码。

知识同步（纪律 7）：需求/方案档发生语义级变更的同一轮交付里，必须同步更新 sources 命中的 platform-knowledge/ 模块导读页（每篇头部有源指针 frontmatter）。

## 部署与运行（16 号节选）

服务器单机部署（Docker Compose，推荐起步）：cd deploy && docker compose up -d --build；
需要知识库向量检索时加挂 qdrant profile。三个服务均为 SQLite 单文件存储，replicaCount 必须为 1。

服务间编址契约表（REQ-518/M325——编排 env 与代码 getenv 对账单源）：
backend → runtime-manager 走 RUNTIME_MGR_URL；backend → runtime-manager MCP 面走 ONTOLOGY_MCP_URL
（缺省自 RUNTIME_MGR_URL 派生 /mcp 路径）；backend → ontology-service 走 BUILD_SVC_URL；
ontology-service → backend 走 PLATFORM_URL；runtime-manager → backend 走 PLATFORM_API_URL。

非本机部署安全边界：平台准入只覆盖 :8080——PLATFORM_TOKEN（accessAuth）仅管 backend 的 /api/*；
ontology-service :8091 与 runtime-manager :8090 完全无鉴权，绝不可暴露公网或不受信网段。
REQ-383 加固后两卫星服务默认监听 127.0.0.1，容器形态显式 ADDR 保持互通。

应用内备份（REQ-411/REQ-517）：设置页「数据与安全」一键备份产出 zip——三库 VACUUM INTO 一致性
快照（platform/ontology/runtime）+ data/projects 项目文件 + .secret 连接密钥 + 伴生快照 +
agent-files。压缩包根 manifest.json 记录生成时间与跳过项；主库快照失败返回 HTTP 500。

## 需求管理与交付纪律（节选）

REQ 编号的唯一分配权威是 docs/18_REQ编号注册表：新需求立项前必须先查本表分配编号；
领号后当轮在 docs/19_REQ状态索引 加行、交付或关闭当轮同步其状态（人读/机读分层，REQ-499）。
交付详录单源（G-7/REQ-375）：交付细节全量只落两处——对应档迭代表与 docs/20 冒烟清单。
pre-commit 门禁：责任批交付 commit 前必须跑 doccheck（backend/internal/doccheck），红即同轮补落。
`

// RealQuestions 真实语料 gold 题（20 题；gold=内容子串，词法臂可命中）。
// 出题面覆盖：编号类（REQ-116/188/499/518/383/411/517）、术语类（目录驱动/数字前缀/
// VACUUM INTO/manifest.json）、机制类（知识同步纪律 7/单源两处/doccheck 门禁）、
// 编址类（五个环境变量）。真实文档结构（引用块/列表/代码段）下的检索质量量化。
var RealQuestions = []struct {
	ID   string
	Q    string
	Gold []string
}{
	{"rc-1", "平台知识页面的收录与归属由什么驱动", []string{"目录驱动"}},
	{"rc-2", "组序和组内序由什么决定", []string{"数字前缀"}},
	{"rc-3", "新增目录或文档后如何上页", []string{"自动入位"}},
	{"rc-4", "REQ-188 规范约束的是什么", []string{"数字前缀"}},
	{"rc-5", "知识同步纪律要求语义级变更时同步什么", []string{"模块导读页"}},
	{"rc-6", "平台知识导读页头部有什么用于溯源", []string{"源指针"}},
	{"rc-7", "服务器单机部署推荐使用什么方式", []string{"Docker Compose"}},
	{"rc-8", "知识库向量检索需要加挂什么", []string{"qdrant"}},
	{"rc-9", "三个服务都是什么存储形态因而副本数必须为一", []string{"SQLite"}},
	{"rc-10", "backend 到 runtime-manager 的 MCP 面走哪个环境变量", []string{"ONTOLOGY_MCP_URL"}},
	{"rc-11", "ONTOLOGY_MCP_URL 缺省时如何派生", []string{"RUNTIME_MGR_URL"}},
	{"rc-12", "runtime-manager 回调 backend 用哪个环境变量", []string{"PLATFORM_API_URL"}},
	{"rc-13", "平台准入覆盖哪个端口", []string{":8080"}},
	{"rc-14", "REQ-383 加固后两卫星服务默认监听什么地址", []string{"127.0.0.1"}},
	{"rc-15", "一键备份的 zip 里三库快照用什么方式生成", []string{"VACUUM INTO"}},
	{"rc-16", "备份压缩包根的清单文件叫什么", []string{"manifest.json"}},
	{"rc-17", "连接密钥在备份 zip 中的路径", []string{".secret"}},
	{"rc-18", "REQ 编号的唯一分配权威是哪份文档", []string{"docs/18_REQ编号注册表"}},
	{"rc-19", "领号后当轮要在哪份机读索引加行", []string{"docs/19_REQ状态索引"}},
	{"rc-20", "交付 commit 前必须跑什么门禁", []string{"doccheck"}},
}
