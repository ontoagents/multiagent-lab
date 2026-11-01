import { useEffect, useRef, useState } from 'react'
import { Button, Checkbox, Input, Modal, Space, Tag, Typography, Upload } from 'antd'
import { DeleteOutlined, FileAddOutlined } from '@ant-design/icons'
import { api, ApiError } from '../../../api/client'
import type { ConceptCandidate, Spec } from '../../../api/types'
import { useUI } from '../../../store/ui'
import { invalidateOntos } from '../shared/useOntoList'

// ---------------------------------------------------------------------------
// REQ-508/M316「从材料开始」学习开局·概念停车场（100 号 R9；承 Q-23 衍生④转正——冷启动建模）：
// BuildPathGrid 新手条第三种起步——粘贴/上传学习材料 → AI 铺候选概念清单（列表形态逐条带
// 材料原句引用，非画布不触 Q-7）→ 逐条勾选/改名 → 一键成骨架（createOntology+saveSpec）
// 进 CustomBuildFlow S2（经 eino.onto.build.customOnto/customStep 键跳转，S2 形态=既有编辑步；
// 结构优先双形态随 REQ-501）。复用：OntoChat 阶段②确认卡勾选/改名行形态 + 阶段③文件导入
// ≤2MB 通道；后端 ai-concept-park 异步 job（aiDraftJob 同口轮询）。
// REQ-573/M381 冷启动建模体验（定位胜负手——四段串联单动线，逐段既有能力复用不新建引擎）：
// 段③骨架生成升级=确认后经 ai-draft-async 既有链补结构（层级/关系），**概念锚定人工确认集**
// （交集合并：concepts=勾选集、relations/parents 仅取两端都在勾选集内的 AI 结果——LLM 不稀释
// 人工确认铁律 REQ-269）；AI 失败/超时 90s 回退平铺骨架如实提示。段④KB 同步入库=勾选后
// createKB（kg_ontology_id 挂新本体=KG 抽取约束词表联动）+uploadKBDoc 收录材料原文——失败不
// 阻断本体交付（best-effort 如实提示）。
// 基底：be34f40c（本轮领取时声明，防并行覆写）。
// ---------------------------------------------------------------------------

/** 停车场行 = 候选 + 人工勾选态（沿 CqCandidateCard CqDraftRow 形态） */
type ParkRow = ConceptCandidate & { checked: boolean }

const MATERIAL_MAX_BYTES = 2 * 1024 * 1024
const MATERIAL_MAX_CHARS = 30000
/** REQ-573 段③ aiDraft 补结构轮询上限（约 90s——超时回退平铺骨架不阻塞交付） */
const STRUCTURE_POLL_TICKS = 45

export default function ConceptParkingFlow({ open, onClose, onCreated }: { open: boolean; onClose: () => void; onCreated: (ontologyId: string) => void }) {
  const { showToast } = useUI()
  const [stage, setStage] = useState<'input' | 'list'>('input')
  const [material, setMaterial] = useState('')
  const [rows, setRows] = useState<ParkRow[]>([])
  const [ontoName, setOntoName] = useState('')
  const [truncated, setTruncated] = useState(0)
  const [progress, setProgress] = useState('')
  const [busy, setBusy] = useState(false)
  const [creating, setCreating] = useState(false)
  const [creatingLabel, setCreatingLabel] = useState('创建骨架')
  // REQ-573 段④：KB 同步入库（默认开——「KB 接近零努力」；失败不阻断）
  const [withKB, setWithKB] = useState(true)
  // 轮询票据：关闭弹窗/重新生成即作废在途轮询（组件常驻，open 切换不重置已铺清单）
  const pollTicket = useRef(0)
  const openRef = useRef(open)
  useEffect(() => {
    openRef.current = open
    if (!open) pollTicket.current++
  }, [open])

  const pickFile = (file: File) => {
    if (file.size > MATERIAL_MAX_BYTES) {
      showToast('文件超过 2MB 上限：请摘取核心段落或拆分后粘贴', 'err')
      return false
    }
    file
      .text()
      .then((t) => setMaterial((m) => (m.trim() ? m + '\n' : '') + `[${file.name}]\n${t}`))
      .catch(() => showToast('文件读取失败', 'err'))
    return false // 阻止 antd Upload 自动上传（沿 OntoChatFlow importFile 先例）
  }

  const generate = async () => {
    const mat = material.trim()
    if (!mat) {
      showToast('请先粘贴或上传学习材料', 'err')
      return
    }
    if ([...mat].length > MATERIAL_MAX_CHARS) {
      showToast(`材料过长（${[...mat].length} 字，上限 ${MATERIAL_MAX_CHARS}）——请分段粘贴或摘取核心段落`, 'err')
      return
    }
    setBusy(true)
    setProgress('')
    try {
      const { job_id } = await api.aiConceptPark(mat)
      const ticket = ++pollTicket.current
      const tick = async () => {
        if (ticket !== pollTicket.current || !openRef.current) return
        try {
          const j = await api.aiDraftJob(job_id)
          if (j.status === 'done') {
            const list = (j.concepts ?? []).map((c) => ({ ...c, checked: true }))
            if (!list.length) {
              showToast('未铺出候选概念，请补充或更换学习材料后重试', 'err')
              setBusy(false)
              return
            }
            setRows(list)
            setOntoName(j.topic?.trim() || '')
            setTruncated(j.truncated ?? 0)
            setStage('list')
            setBusy(false)
            return
          }
          if (j.status === 'failed') {
            showToast(j.error || '铺候选失败', 'err')
            setBusy(false)
            return
          }
          setProgress(j.progress || '生成中')
          window.setTimeout(tick, 1500)
        } catch (e: any) {
          showToast(e?.message ?? '轮询失败', 'err')
          setBusy(false)
        }
      }
      window.setTimeout(tick, 800)
    } catch (e: any) {
      showToast(e?.message ?? '铺候选失败', 'err')
      setBusy(false)
    }
  }

  const pickedCount = rows.filter((r) => r.checked && r.name.trim()).length

  // REQ-573 段③：aiDraft 补结构（best-effort）——返回合并后 relations/parents 映射；失败/超时返 null
  const draftStructure = async (picked: ParkRow[]): Promise<{ relations: Spec['relations']; parents: Map<string, string[]> } | null> => {
    const names = picked.map((r) => r.name.trim())
    const description = [ontoName.trim() || '材料起步本体', material.trim().slice(0, 2000)].filter(Boolean).join('\n')
    const hint = `学习者已在概念停车场人工确认以下概念为最终概念清单（共 ${names.length} 个）：${names.join('、')}。请只围绕这些概念补全建模：给出概念间关系（relations）与上下位层级（parents）；不要新增清单之外的概念。`
    try {
      const { job_id } = await api.aiDraftOntologyAsync(description, hint)
      const ticket = pollTicket.current
      for (let i = 0; i < STRUCTURE_POLL_TICKS; i++) {
        if (ticket !== pollTicket.current || !openRef.current) return null
        await new Promise((r) => window.setTimeout(r, 2000))
        const j = await api.aiDraftJob(job_id)
        if (j.status === 'done' && j.result?.spec) {
          const nameSet = new Set(names)
          const draft = j.result.spec
          const relations = (draft.relations ?? []).filter((rel) => nameSet.has(rel.from) && nameSet.has(rel.to))
          const parents = new Map<string, string[]>()
          for (const c of draft.concepts ?? []) {
            const ps = (c.parents ?? []).filter((p) => nameSet.has(p) && p !== c.name)
            if (ps.length) parents.set(c.name, ps)
          }
          return { relations, parents }
        }
        if (j.status === 'failed') return null
      }
      return null // 超时
    } catch {
      return null
    }
  }

  const createSkeleton = async () => {
    const picked = rows.filter((r) => r.checked && r.name.trim())
    if (!picked.length) {
      showToast('请至少勾选一个概念', 'err')
      return
    }
    setCreating(true)
    let kbNote = ''
    try {
      const nm = ontoName.trim() || '材料起步本体'
      // REQ-573 段③：AI 补结构（失败/超时/弹窗关闭=回退平铺骨架，如实提示不冒充）
      setCreatingLabel('AI 补全层级与关系中…')
      const structure = await draftStructure(picked)
      const concepts = picked.map((r) => {
        const ps = structure?.parents.get(r.name.trim()) ?? []
        return {
          name: r.name.trim(),
          label: r.label?.trim() || undefined,
          definition: r.definition?.trim() || undefined,
          ...(ps.length ? { parents: ps } : {}),
        }
      })
      const relations = structure?.relations ?? []
      const spec: Spec = {
        name: nm,
        description: material.trim().slice(0, 120) || undefined,
        // 原句引用（origin）仅停车场呈现面，不入 spec——概念级备注随版本留痕归 REQ-510
        concepts,
        relations,
        instances: [],
      }
      setCreatingLabel('创建本体…')
      const created = await api.createOntology({ name: nm, description: spec.description })
      await api.saveSpec(created.id, spec)
      invalidateOntos() // REQ-467：单源清单失效广播
      // REQ-573 段④：KB 同步入库（best-effort——失败不阻断本体交付；同名库复用不重建造免 409）
      if (withKB) {
        try {
          setCreatingLabel('创建知识库并收录材料…')
          const kbs = await api.listKBs()
          const existing = (kbs ?? []).find((k) => k.name === nm)
          const kb = existing
            ? await api.updateKB(existing.id, { ...existing, kg_ontology_id: created.id } as any) // PUT 全量更新——带既有字段防零值覆盖
            : await api.createKB({ name: nm, description: `冷启动动线随本体创建（REQ-573）；KG 抽取约束=${nm}`, kg_ontology_id: created.id } as any)
          await api.uploadKBDoc(kb.id, { name: `${nm} · 学习材料`, content: material.trim() })
          kbNote = `；知识库「${nm}」${existing ? '已复用（挂本本体为 KG 约束）并' : '已建并'}收录材料`
        } catch (e: any) {
          kbNote = `；知识库入库失败（${e?.message ?? '未知错误'}）——可稍后在知识库页手动创建`
        }
      }
      const structNote = relations.length
        ? `，AI 补全 ${relations.length} 条关系${structure ? '' : ''}`
        : structure
          ? '（AI 未补出关系，为概念清单形态，可在编辑页继续补）'
          : '（AI 结构补全未成功，为概念清单形态，可在编辑页继续补）'
      showToast(`骨架已创建（${picked.length} 个概念${structNote}）${kbNote}，进入编辑`)
      onClose()
      onCreated(created.id)
    } catch (e: any) {
      if (e instanceof ApiError && e.validationErrors?.length) showToast(`骨架校验未通过：${e.validationErrors[0].message}`, 'err')
      else showToast(e?.message ?? '创建失败', 'err')
    } finally {
      setCreating(false)
      setCreatingLabel('创建骨架')
    }
  }

  return (
    <Modal
      open={open}
      onCancel={onClose}
      width={680}
      title="从材料开始 · 概念停车场"
      footer={
        stage === 'input' ? (
          <Space>
            <Button onClick={onClose}>取消</Button>
            <Button type="primary" loading={busy} disabled={!material.trim()} onClick={generate} data-testid="material-generate">
              AI 铺候选概念
            </Button>
          </Space>
        ) : (
          <Space>
            <Button onClick={() => setStage('input')}>返回改材料</Button>
            <Button type="primary" loading={creating} disabled={pickedCount === 0} onClick={createSkeleton} data-testid="material-create-skeleton">
              {creating ? creatingLabel : `用 ${pickedCount} 个概念创建骨架`}
            </Button>
          </Space>
        )
      }
    >
      {stage === 'input' ? (
        <>
          <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 8 }}>
            粘贴或上传一段学习材料（≤2MB / 3 万字），AI 铺出候选概念清单——逐条勾选、改名后一键生成本体骨架（AI 顺手补全层级与关系）并可同步创建知识库收录材料；清单仅为候选，最终以人工确认为准。
          </Typography.Paragraph>
          <Input.TextArea
            data-testid="material-input"
            rows={8}
            value={material}
            maxLength={MATERIAL_MAX_CHARS}
            showCount
            onChange={(e) => setMaterial(e.target.value)}
            placeholder="例：一段课程笔记、教科书节选、技术文档……"
          />
          <Space style={{ marginTop: 8 }} size={8}>
            <Upload accept=".md,.markdown,.yaml,.yml,.txt,.json,.csv,.log" showUploadList={false} beforeUpload={pickFile}>
              <Button size="small" icon={<FileAddOutlined />} data-testid="material-upload">
                上传材料（≤2MB）
              </Button>
            </Upload>
            {busy && progress && (
              <Typography.Text type="secondary" style={{ fontSize: 12 }} data-testid="material-progress">
                {progress}
              </Typography.Text>
            )}
          </Space>
        </>
      ) : (
        <>
          <Space style={{ marginBottom: 8, width: '100%' }} direction="vertical" size={4}>
            <Space size={8} wrap>
              <Typography.Text style={{ fontSize: 12.5 }}>本体名称</Typography.Text>
              <Input size="small" style={{ width: 260 }} value={ontoName} onChange={(e) => setOntoName(e.target.value)} placeholder="材料起步本体" data-testid="material-onto-name" />
              {truncated > 0 && <Tag style={{ fontSize: 11 }}>已去重/截断 {truncated} 条（上限 30）</Tag>}
            </Space>
            <Typography.Text type="secondary" style={{ fontSize: 11.5 }}>
              勾选 {pickedCount}/{rows.length} 条 · 灰字为 AI 定义与材料原句引用（原句仅此呈现，不入骨架）· 创建时 AI 按勾选清单补全层级与关系（概念以人工勾选为准）
            </Typography.Text>
          </Space>
          <div data-testid="material-candidates" style={{ maxHeight: 360, overflowY: 'auto' }}>
            {rows.map((r, i) => (
              <div key={i} data-testid={`material-candidate-row-${i}`} style={{ display: 'flex', gap: 6, alignItems: 'flex-start', marginBottom: 6 }}>
                <Checkbox checked={r.checked} onChange={(e) => setRows(rows.map((r2, j) => (j === i ? { ...r2, checked: e.target.checked } : r2)))} style={{ marginTop: 5 }} />
                <div style={{ flex: 1 }}>
                  <Input size="small" value={r.name} onChange={(e) => setRows(rows.map((r2, j) => (j === i ? { ...r2, name: e.target.value } : r2)))} placeholder="概念名" />
                  {(r.definition || r.origin) && (
                    <Typography.Paragraph type="secondary" style={{ fontSize: 11, margin: '2px 0 0', lineHeight: 1.5 }}>
                      {r.definition}
                      {r.origin ? ` · 「${r.origin}」` : ''}
                    </Typography.Paragraph>
                  )}
                </div>
                <Button size="small" type="text" icon={<DeleteOutlined />} aria-label="删除候选" onClick={() => setRows(rows.filter((_, j) => j !== i))} />
              </div>
            ))}
          </div>
          <Space style={{ marginTop: 4 }} size={12}>
            <Button size="small" onClick={() => setRows([...rows, { name: '', checked: true }])}>
              手动添加
            </Button>
            {/* REQ-573 段④：KB 同步入库开关（默认开；失败不阻断本体交付） */}
            <Checkbox checked={withKB} onChange={(e) => setWithKB(e.target.checked)} data-testid="material-with-kb">
              同时创建知识库并收录材料（KG 抽取挂本本体为约束）
            </Checkbox>
          </Space>
        </>
      )}
    </Modal>
  )
}
