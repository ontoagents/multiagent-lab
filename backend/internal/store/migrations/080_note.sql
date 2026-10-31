-- REQ-495/M304 笔记本 Notes（自产成果文档资产层——Q-23 定位闭环最后一块；详设 11 §2 REQ-495 行+三拍板）。
-- 绑定宿主=新 workspace 实体（Q-24 实体反转修订注——REQ-548 M1a 已先行，notebook.workspace_id
-- REFERENCES workspace(id) ON DELETE SET NULL 零返工：删工作区笔记本回落个人资产、页面保留）。
CREATE TABLE IF NOT EXISTS notebook (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  description TEXT NOT NULL DEFAULT '',
  workspace_id TEXT NULL REFERENCES workspace(id) ON DELETE SET NULL,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
-- 页面两级树第二级：source=manual（手写）| local（本地目录源）| git（git 文档仓库源，本地克隆路径 pull+walk）；
-- source_ref=源内相对路径（目录源）或克隆路径；source_mtime=触碰同步基线（mtime 相同跳过）。
CREATE TABLE IF NOT EXISTS note_page (
  id TEXT PRIMARY KEY,
  notebook_id TEXT NOT NULL REFERENCES notebook(id) ON DELETE CASCADE,
  title TEXT NOT NULL,
  content_md TEXT NOT NULL DEFAULT '',
  source TEXT NOT NULL DEFAULT 'manual',
  source_ref TEXT NOT NULL DEFAULT '',
  source_mtime TEXT NOT NULL DEFAULT '',
  position INTEGER NOT NULL DEFAULT 0,
  created_at TEXT NOT NULL,
  updated_at TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_note_page_nb ON note_page(notebook_id);
-- ④ KB 回流=引用语义（三拍板③）：KB 登记笔记本为语料源（笔记本保持单源不拷贝），
-- 增量随触碰同步——页更新/删除时 handler 对引用 KB 重索引（delete+ImportAsync 确定性标题）。
CREATE TABLE IF NOT EXISTS kb_notebook_source (
  id TEXT PRIMARY KEY,
  kb_id TEXT NOT NULL REFERENCES knowledge_base(id) ON DELETE CASCADE,
  notebook_id TEXT NOT NULL REFERENCES notebook(id) ON DELETE CASCADE,
  created_at TEXT NOT NULL,
  UNIQUE(kb_id, notebook_id)
);
