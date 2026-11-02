import { useEffect, useMemo, useState } from 'react'
import { Button, Input, Modal, Select, Typography } from 'antd'
import { api } from '../api/client'
import type { Notebook, Project, Workspace } from '../api/types'
import { useUI } from '../store/ui'

/**
 * REQ-495②段尾项（01 §3.15②/18A REQ-495 诚实边界①收口）：「保存到笔记本」产物去向动作。
 * 目标解析链（沿 11 §2 REQ-495 三拍板②：工作区↔笔记本一对多，笔记本=自产成果资产层）：
 *   项目归属工作区（REQ-550）→ 该工作区绑定的笔记本 → 写入 note_page（manual 源）。
 * 未归属工作区 / 工作区未绑定笔记本 → 明确引导指向绑定入口（不静默失败）。
 * 视觉语言沿 REQ-488~494 静音批（灰字、无彩色 Tag）。
 * 边界：笔记本编辑器与容器/页面 CRUD 本身归 NotesPage（REQ-495 已交付），本组件只做去向写入。
 */

/** 大文件上限：与文件预览口径一致（≤1MB 文本读取；二进制/超限不进笔记本） */
const MAX_SAVE = 1024 * 1024

/** 可作笔记本页面正文的文本类扩展名（二进制存文本会乱码，如实拒引下载通道） */
const TEXT_EXTS = new Set([
  'md', 'markdown', 'txt', 'json', 'csv', 'html', 'htm', 'xml', 'yaml', 'yml', 'log',
  'ts', 'tsx', 'js', 'jsx', 'css', 'less', 'scss', 'py', 'go', 'java', 'rs', 'rb', 'php',
  'sh', 'bat', 'ps1', 'sql', 'toml', 'ini', 'conf', 'env', 'proto', 'graphql', 'dockerfile',
])

const isTextFile = (name: string) => {
  const ext = name.includes('.') ? name.split('.').pop()!.toLowerCase() : ''
  return TEXT_EXTS.has(ext) || ext === ''
}

/** 跨组件跳转绑定入口：切到「工作区」页签并聚焦指定工作区（ProjectsPage/WorkspacePage 监听） */
export const gotoWorkspaceBinding = (workspaceId?: string) => {
  try {
    if (workspaceId) sessionStorage.setItem('eino.focus.workspace', workspaceId)
    else sessionStorage.removeItem('eino.focus.workspace')
  } catch { /* 配额满静默（沿 REQ-509 口径） */ }
  window.dispatchEvent(new CustomEvent('eino:goto.projtab'))
}

/** REQ-580/M395：项目侧深链协议（eino.focus.project）——切到「项目」页签并聚焦指定项目的侧板配置视图
 *  （ProjectsPage 监听：选中该项目+开侧板+config 视图；未归属分支两步引导第一步直达） */
export const gotoProjectSettings = (projectId?: string) => {
  try {
    if (projectId) sessionStorage.setItem('eino.focus.project', projectId)
    else sessionStorage.removeItem('eino.focus.project')
  } catch { /* 配额满静默（沿 REQ-509 口径） */ }
  window.dispatchEvent(new CustomEvent('eino:goto.projtab.project'))
}

export interface SaveTargetFile {
  name: string
  path?: string
  size?: number
  /** REQ-586/M401：产物 file_id（artifact.saved 聚合携带——工作区直挂模式 content 反查链） */
  file_id?: string
}

export default function SaveToNotebookModal({
  project,
  workspace,
  file,
  onClose,
}: {
  /** REQ-495②：项目会话模式（归属工作区解析绑定） */
  project?: Project
  /** REQ-586/M401：工作区直挂模式（workspace_id 直解绑定——跳过归属步骤） */
  workspace?: Workspace
  file: SaveTargetFile
  onClose: () => void
}) {
  const { showToast, bumpData } = useUI()
  const [phase, setPhase] = useState<'resolving' | 'guide' | 'form'>('resolving')
  const [guide, setGuide] = useState<{ title: string; body: string; focusWorkspace?: string; focusProject?: string; steps?: string[] } | null>(null)
  const [bound, setBound] = useState<Notebook[]>([])
  const [nbId, setNbId] = useState('')
  const [title, setTitle] = useState(file.name)
  const [saving, setSaving] = useState(false)
  const [fatal, setFatal] = useState('')

  const tooBig = useMemo(() => typeof file.size === 'number' && file.size > MAX_SAVE, [file.size])
  const notText = useMemo(() => !isTextFile(file.name), [file.name])

  useEffect(() => {
    let alive = true
    ;(async () => {
      // REQ-586/M401：工作区直挂模式——workspace_id 直解（跳过归属步骤）
      if (workspace) {
        const nbs = await api.listNotebooks(workspace.id).catch(() => [])
        if (!alive) return
        if (nbs.length === 0) {
          setGuide({
            title: '当前工作区尚未绑定笔记本',
            body: '产物保存到笔记本由工作区的笔记本绑定决定：请在右侧侧板「配置」视图的「笔记本绑定」区块绑定后重试。',
            focusWorkspace: workspace.id,
          })
          setPhase('guide')
          return
        }
        setBound(nbs)
        setNbId(nbs[0]?.id ?? '')
        setPhase('form')
        return
      }
      // ① 解析工作区：项目归属（REQ-550）优先；未归属 → 引导先归属（两模式互斥下 project 必在）
      if (!project) {
        if (alive) setFatal('缺少保存目标上下文（无项目/工作区）')
        return
      }
      const wsId = project.workspace_id ?? ''
      if (!wsId) {
        if (!alive) return
        setGuide({
          title: '先归属工作区，再保存到笔记本',
          body: `项目「${project?.name ?? ''}」尚未归属工作区。笔记本按工作区绑定解析保存目标：
第一步：把本项目归属到一个工作区（下方按钮直达本项目侧板「配置」视图的归属设置）；
第二步：到「项目」模块「工作区」页签选中该工作区，在右侧侧板「配置」视图绑定笔记本（完成后重试保存）。`,
          focusProject: project?.id,
          steps: ['去项目设置归属', '绑定后重试'],
        })
        setPhase('guide')
        return
      }
      // ② 工作区绑定清单：空 → 引导绑定入口
      let nbs: Notebook[] = []
      try {
        nbs = await api.listNotebooks(wsId)
      } catch (e: any) {
        if (!alive) return
        setFatal(e?.message ?? '笔记本清单读取失败')
        setPhase('guide')
        return
      }
      if (!alive) return
      if (nbs.length === 0) {
        setGuide({
          title: '当前工作区尚未绑定笔记本',
          body: `产物保存到笔记本由工作区的笔记本绑定决定：请到「项目」模块「工作区」页签选中目标工作区，在右侧侧板「配置」视图的「笔记本绑定」区块绑定后重试。`,
          focusWorkspace: wsId,
        })
        setPhase('guide')
        return
      }
      setBound(nbs)
      setNbId(nbs[0]?.id ?? '')
      setPhase('form')
    })()
    return () => {
      alive = false
    }
  }, [project?.id, project?.workspace_id, project?.name])

  const save = async () => {
    if (!workspace && !project) return // 两模式互斥兜底（TS 收窄）
    const nb = bound.find((x) => x.id === nbId)
    if (!nb) return
    const t = title.trim()
    if (!t) {
      showToast('页面标题必填', 'err')
      return
    }
    setSaving(true)
    try {
      // REQ-586/M401：工作区直挂模式——file_id 反查 project_id 后经项目 dir-file 读盘（content 链）
      let content: string
      if (workspace) {
        if (!file.path) {
          showToast('产物无路径信息，无法读取内容', 'err')
          setSaving(false)
          return
        }
        const meta = await api.getProjectFileByID(file.file_id ?? '')
        content = await (await fetch(`/api/projects/${meta.project_id}/dir-file?path=${encodeURIComponent(meta.path)}`)).text()
      } else {
        content = await api.getProjectDirFile(project!.id, file.path!)
      }
      const page = await api.createNotePage(nb.id, { title: t, content_md: content, source: 'manual' })
      showToast(`已保存到笔记本「${nb.name}」：页面「${page.title}」（可在「知识库」模块笔记本页签查看）`)
      bumpData()
      onClose()
    } catch (e: any) {
      showToast(e?.message ?? '保存失败', 'err')
    } finally {
      setSaving(false)
    }
  }

  return (
    <Modal
      title="保存到笔记本"
      open
      onCancel={onClose}
      width={480}
      footer={
        phase === 'form' ? [
          <Button key="cancel" size="small" onClick={onClose}>取消</Button>,
          <Button
            key="save"
            size="small"
            type="primary"
            loading={saving}
            disabled={tooBig || notText}
            data-testid="save-notebook-btn"
            onClick={save}
          >
            保存
          </Button>,
        ] : [
          <Button key="close" size="small" onClick={onClose}>知道了</Button>,
          ...(guide && (phase === 'guide'))
            ? [
                guide.focusProject ? (
                  <Button
                    key="goto-proj"
                    size="small"
                    type="primary"
                    data-testid="goto-proj-btn"
                    onClick={() => {
                      gotoProjectSettings(guide.focusProject)
                      onClose()
                    }}
                  >
                    去项目设置归属
                  </Button>
                ) : (
                  <Button
                    key="goto"
                    size="small"
                    type="primary"
                    data-testid="goto-bind-btn"
                    onClick={() => {
                      gotoWorkspaceBinding(guide.focusWorkspace)
                      onClose()
                    }}
                  >
                    前往绑定
                  </Button>
                ),
              ]
            : [],
        ]
      }
    >
      {phase === 'resolving' && (
        <Typography.Text type="secondary" style={{ fontSize: 12.5 }} data-testid="save-resolving">
          正在解析保存目标…
        </Typography.Text>
      )}
      {phase === 'guide' && guide && (
        <div data-testid="save-notebook-guide">
          <Typography.Text style={{ fontSize: 13, display: 'block', marginBottom: 8 }}>{guide.title}</Typography.Text>
          <Typography.Text type="secondary" style={{ fontSize: 12.5, display: 'block', lineHeight: 1.7 }}>
            {guide.body}
          </Typography.Text>
          {fatal && (
            <Typography.Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 8 }}>
              {fatal}
            </Typography.Text>
          )}
        </div>
      )}
      {phase === 'form' && (
        <div data-testid="save-notebook-form">
          <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block', marginBottom: 10 }}>
            文件「{file.name}」{file.path !== file.name ? `（${file.path}）` : ''} 将作为新页面写入所选笔记本（manual 源，保存后可自由编辑）。
          </Typography.Text>
          <div style={{ marginBottom: 10 }}>
            <Typography.Text style={{ fontSize: 12.5, display: 'block', marginBottom: 4 }}>目标笔记本</Typography.Text>
            <Select
              size="small"
              style={{ width: '100%' }}
              value={nbId || undefined}
              onChange={(v) => setNbId(v ?? '')}
              options={bound.map((n) => ({ value: n.id, label: n.name }))}
              data-testid="save-notebook-select"
            />
          </div>
          <div>
            <Typography.Text style={{ fontSize: 12.5, display: 'block', marginBottom: 4 }}>页面标题</Typography.Text>
            <Input
              size="small"
              value={title}
              onChange={(e) => setTitle(e.target.value)}
              onPressEnter={save}
              data-testid="save-notebook-title"
            />
          </div>
          {(tooBig || notText) && (
            <Typography.Text type="danger" style={{ fontSize: 12, display: 'block', marginTop: 10 }} data-testid="save-notebook-guard">
              {tooBig
                ? `文件超过 1MB（${Math.ceil((file.size ?? 0) / 1024)} KB），暂不支持存入笔记本（与文件预览同口径）。`
                : '二进制类文件暂不支持存入笔记本（正文会乱码）——请走下载通道。'}
            </Typography.Text>
          )}
        </div>
      )}
    </Modal>
  )
}
