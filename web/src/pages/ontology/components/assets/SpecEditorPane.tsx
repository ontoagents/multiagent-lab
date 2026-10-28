import { useEffect, useState } from 'react'
import { Alert, Button, Collapse, Form, Input, Modal, Space, Table, Tag, Tooltip, Typography } from 'antd'
import { DownloadOutlined, ReloadOutlined } from '@ant-design/icons'
import { api, ApiError } from '../../../../api/client'
import type { Ontology, OverlayRow, Spec, ValidationError } from '../../../../api/types'
import { useUI } from '../../../../store/ui'
import { ERR_COLUMNS, LARGE_SPEC_CHARS, ReloadHintAlert } from '../../shared'
import JsonEditor from '../JsonEditor'

// ---------------------------------------------------------------------------
// Spec 编辑（原 S2：元信息 + JSON 编辑器 + 保存校验）。
// A1（REQ-145/M22）：TextArea → CodeMirror JSON 模式（语法高亮 + jsonParseLinter
// 行内错误标记）；保存路径保留 JSON.parse try/catch 与后端校验错误表格。
// ---------------------------------------------------------------------------

export function emptySpec(name: string): Spec {
  return { name, description: '', concepts: [], relations: [], instances: [] }
}

export default function SpecEditorPane({
  ontology,
  spec,
  specLoading,
  specErr,
  onReloadSpec,
  onMetaSaved,
  onSpecSaved,
  onDirtyChange,
}: {
  ontology: Ontology
  spec: Spec | null
  specLoading: boolean
  specErr: string | null
  onReloadSpec: () => void
  onMetaSaved: () => void
  onSpecSaved: (version: number) => void
  /** REQ-466/M266：脏标记上报（AssetsPage 据此挂离开防护） */
  onDirtyChange?: (dirty: boolean) => void
}) {
  const { showToast } = useUI()
  const [metaForm] = Form.useForm()
  const [specText, setSpecText] = useState('')
  const [savingMeta, setSavingMeta] = useState(false)
  const [savingSpec, setSavingSpec] = useState(false)
  const [validationErrors, setValidationErrors] = useState<ValidationError[]>([])
  const [lastVersion, setLastVersion] = useState<number | null>(null)
  // REQ-253②（59 号 P2）：缩进大纲→概念层级批量导入（教学场景：讲义大纲直接建层级）
  const [outlineOpen, setOutlineOpen] = useState(false)
  const [outlineText, setOutlineText] = useState('')
  // REQ-304：大 Spec 默认不挂编辑器（摘要卡），用户显式打开才挂只读全文
  const [forceEditor, setForceEditor] = useState(false)

  useEffect(() => {
    metaForm.setFieldsValue({ name: ontology.name, description: ontology.description ?? '' })
  }, [ontology.id, ontology.name, ontology.description, metaForm])

  useEffect(() => {
    setSpecText(spec ? JSON.stringify(spec, null, 2) : '')
    setValidationErrors([])
    setForceEditor(false)
  }, [spec])

  // REQ-466/M266 编辑安全感：脏标记=编辑文本偏离已加载基线（spec prop 序列化）。
  // 常驻保活只护住「切分区」，刷新/关闭与切本体的丢失路径由此补齐。
  const baseline = spec ? JSON.stringify(spec, null, 2) : ''
  const dirty = specText !== '' && specText !== baseline

  useEffect(() => {
    onDirtyChange?.(dirty)
    // eslint-disable-next-line react-hooks/exhaustive-deps -- onDirtyChange 为 setState 稳定引用
  }, [dirty])

  // 刷新/关闭前原生确认（仅脏态挂监听）
  useEffect(() => {
    if (!dirty) return
    const h = (e: BeforeUnloadEvent) => {
      e.preventDefault()
      e.returnValue = ''
    }
    window.addEventListener('beforeunload', h)
    return () => window.removeEventListener('beforeunload', h)
  }, [dirty])

  /** 重新加载守卫：脏态下确认后才用服务端版本覆盖本地修改 */
  const reloadGuarded = () => {
    if (!dirty) {
      onReloadSpec()
      return
    }
    Modal.confirm({
      title: '重新加载将覆盖未保存的修改',
      content: '「Spec JSON」有未保存的本地修改，重新加载会以服务端版本覆盖。请确认已备份需要保留的内容。',
      okText: '放弃修改并重载',
      okButtonProps: { danger: true },
      cancelText: '继续编辑',
      onOk: onReloadSpec,
    })
  }

  const large = specText.length > LARGE_SPEC_CHARS

  /** REQ-253②：缩进大纲解析——每行 strip 后为概念名，缩进深度（tab=2 空格）=层级；返回 {name,depth} */
  const parseOutline = (text: string): { name: string; depth: number }[] => {
    const out: { name: string; depth: number }[] = []
    for (const line of text.split('\n')) {
      if (!line.trim()) continue
      const m = line.match(/^(\s*)\S/)
      const indent = m ? (m[1] ?? '').replace(/\t/g, '  ').length : 0
      out.push({ name: line.trim(), depth: Math.floor(indent / 2) })
    }
    return out
  }

  /** 大纲并入当前 spec 草稿（specText）——去重已存在概念；父链按缩进深度相邻回溯；仅改草稿不保存（审查后手动保存） */
  const applyOutline = () => {
    const items = parseOutline(outlineText)
    if (items.length === 0) return
    let parsed: Spec
    try {
      parsed = JSON.parse(specText || '{}')
    } catch {
      showToast('当前 Spec JSON 解析失败，无法并入', 'err')
      return
    }
    parsed.concepts = parsed.concepts ?? []
    const existing = new Set(parsed.concepts.map((c) => c.name))
    const stack: { name: string; depth: number }[] = []
    let added = 0
    for (const it of items) {
      while (stack.length > 0 && stack[stack.length - 1].depth >= it.depth) stack.pop()
      if (!existing.has(it.name)) {
        const parents = stack.length > 0 ? [stack[stack.length - 1].name] : undefined
        parsed.concepts.push(parents ? { name: it.name, parents } : { name: it.name })
        existing.add(it.name)
        added++
      }
      stack.push({ name: it.name, depth: it.depth })
    }
    setSpecText(JSON.stringify(parsed, null, 2))
    setOutlineOpen(false)
    setOutlineText('')
    showToast(`大纲已并入草稿（新增 ${added} 概念；请检查后手动保存）`)
  }

  const saveMeta = async () => {
    let v: any
    try {
      v = await metaForm.validateFields()
    } catch {
      return
    }
    setSavingMeta(true)
    try {
      await api.updateOntologyMeta(ontology.id, { name: v.name, description: v.description ?? '' })
      showToast('基本信息已保存')
      onMetaSaved()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setSavingMeta(false)
    }
  }

  const saveSpec = async () => {
    let parsed: Spec
    try {
      parsed = JSON.parse(specText)
    } catch (e: any) {
      showToast(`JSON 解析失败：${e.message}`, 'err')
      return
    }
    // REQ-249/M72：保存前本地轻量查重（概念重名/概念↔实例跨域同名）——后端 Validate 同口径，
    // 命中即本地呈现可读信息并阻断发送（不必等后端往返；大 Spec 时省一次上传）
    const localErrs: ValidationError[] = []
    const cn = new Set<string>()
    ;(parsed.concepts ?? []).forEach((c, i) => {
      if (cn.has(c.name)) localErrs.push({ path: `concepts[${i}].name`, message: '概念名重复: ' + c.name })
      cn.add(c.name)
    })
    const inNames = new Set<string>()
    ;(parsed.instances ?? []).forEach((it, i) => {
      inNames.add(it.name)
      if (cn.has(it.name)) localErrs.push({ path: `instances[${i}].name`, message: '概念与实例同名: ' + it.name + '（请改名其一）' })
    })
    ;(parsed.instances ?? []).forEach((it) => {
      if (cn.has(it.name)) inNames.add(it.name)
    })
    ;(parsed.instances ?? []).forEach((it, i) =>
      (it.relations ?? []).forEach((ir, j) => {
        if (cn.has(it.name) && !inNames.has(ir.target) && ir.target)
          localErrs.push({ path: `instances[${i}].relations[${j}].target`, message: '引用了未定义实例: ' + ir.target })
      }),
    )
    if (localErrs.length > 0) {
      setValidationErrors(localErrs)
      showToast(`本地查重发现 ${localErrs.length} 处命名问题，未发送保存`, 'err')
      return
    }
    setSavingSpec(true)
    setValidationErrors([])
    try {
      // REQ-293②：If-Match 防陈旧（基于最近已知版本；并发修改 409 不静默覆盖）
      const r = await api.saveSpec(ontology.id, parsed, lastVersion ?? ontology.version)
      showToast(`Spec 已保存（version ${r.version}${r.overlays_recorded ? `，人工编辑保护 ${r.overlays_recorded} 项` : ''}）`)
      setLastVersion(r.version)
      onSpecSaved(r.version)
    } catch (e: any) {
      if (e instanceof ApiError && e.status === 409) {
        showToast(e.message, 'err') // 服务端消息含 当前版本 vs 编辑基线——请刷新后重试
      } else if (e instanceof ApiError && e.validationErrors?.length) {
        setValidationErrors(e.validationErrors)
        showToast('校验未通过，请修正后重试', 'err')
      } else {
        showToast(e.message, 'err')
      }
    } finally {
      setSavingSpec(false)
    }
  }

  return (
    <>
      <Form form={metaForm} layout="vertical" requiredMark={false}>
        <div className="onto-meta-row">
          <Form.Item name="name" label="名称" rules={[{ required: true, message: '名称必填' }]} style={{ width: 260, marginBottom: 0 }}>
            <Input />
          </Form.Item>
          <Form.Item name="description" label="描述" style={{ flex: 1, marginBottom: 0 }}>
            <Input placeholder="本体用途说明" />
          </Form.Item>
          <Button type="primary" loading={savingMeta} onClick={saveMeta}>
            保存基本信息
          </Button>
        </div>
      </Form>

      <div className="onto-sec">
        <span className="onto-sec-title">Spec JSON（concepts / relations / instances 三要素）</span>
        {dirty && (
          <Tooltip title="本地修改尚未保存——刷新/关闭/切换本体会丢失（REQ-466 离开防护已挂）">
            <Tag color="orange" style={{ margin: 0 }} data-testid="spec-dirty-tag">
              未保存
            </Tag>
          </Tooltip>
        )}
        <span className="hit-spacer" />
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {specLoading
            ? '加载中…'
            : spec
              ? `概念 ${spec.concepts?.length ?? 0} · 关系 ${spec.relations?.length ?? 0} · 实例 ${spec.instances?.length ?? 0}`
              : '尚未保存过 Spec'}
        </Typography.Text>
        <Button size="small" icon={<ReloadOutlined />} onClick={reloadGuarded} disabled={specLoading}>
          重新加载
        </Button>
        <Button size="small" onClick={() => setOutlineOpen(true)} data-testid="outline-import-btn">
          大纲导入
        </Button>
        {!spec && !specLoading && (
          <Button size="small" onClick={() => setSpecText(JSON.stringify(emptySpec(ontology.name), null, 2))}>
            初始化空 Spec
          </Button>
        )}
        <Button size="small" type="primary" loading={savingSpec} disabled={!specText || large} onClick={saveSpec}>
          保存 Spec
        </Button>
      </div>

      {/* REQ-248/G2：能力问题（CQ）一等公民面板——手工路径可录可看；写入 spec.cq 随保存入库（仅 spec 层，
          不入 TTL/vowljson/不参与校验）。LLM 三路径已自动回写；此处为手工路径录入点与方法论卡援引落点。 */}
      {spec && (
        <Collapse
          size="small"
          style={{ marginTop: 10 }}
          items={[
            {
              key: 'cq',
              label: `能力问题（CQ）${(spec.cq?.length ?? 0) > 0 ? ` · ${spec.cq!.length} 条` : ' · 未录入'}`,
              children: (
                <CqEditor
                  value={spec.cq ?? []}
                  onChange={(next) => {
                    try {
                      const obj = JSON.parse(specText) as Spec
                      obj.cq = next
                      setSpecText(JSON.stringify(obj, null, 2))
                    } catch {
                      // specText 非法 JSON 时不强改（保存时错误表兜底）
                    }
                  }}
                />
              ),
            },
          ]}
        />
      )}
      {/* REQ-292/M96 编辑三层（base+overlay+audit）：人工编辑保护清单——机器重算（合并/灌装）后人工值重放不丢；
          恢复=撤回人工编辑回到机器产出 base。backend auditOntologyProxy 对编辑/恢复落 onto_decision 审计。 */}
      {spec && <OverlayProtectPanel ontologyId={ontology.id} onChanged={() => onReloadSpec()} />}
      {specErr ? (
        <Alert
          type="error"
          showIcon
          style={{ marginTop: 10 }}
          title="Spec 加载失败"
          description={specErr}
          action={
            <Button size="small" onClick={onReloadSpec}>
              重试
            </Button>
          }
        />
      ) : (
        <>
          {/* REQ-304：大 Spec 默认摘要卡（不挂 CodeMirror——linter 全文 parse + 建树在 MB 级阻塞主线程），
              显式打开才挂只读全文（无 lint，合法性由保存路径兜底）；小 Spec 行为不变（可编辑+lint） */}
          {large && !forceEditor && (
            <LargeSpecSummary
              ontologyId={ontology.id}
              specText={specText}
              spec={spec}
              onOpen={() => setForceEditor(true)}
            />
          )}
          {large && forceEditor && (
            <Alert
              type="warning"
              showIcon
              style={{ margin: '10px 0' }}
              title="Spec 体积较大，已切换为只读"
              description="全文仅供查看（大文档解析可能需要数秒）；对象级编辑请走「图形编辑」，整份修改请在本地编辑后经导入 / 导出接口处理。"
            />
          )}
          {spec && (spec.concepts?.length ?? 0) === 0 && (spec.relations?.length ?? 0) === 0 && (
            <Alert
              type="info"
              showIcon
              style={{ marginTop: 10 }}
              title="从第一个概念开始"
              description="在下方 JSON 的 concepts 数组添加概念（name 必填，label/definition/parents 可选）；或展开「能力问题」先列 3~5 条 CQ 再动手（建模方法论第一步）。也可以到构建栏参考素材面板看种子建模说明。"
            />
          )}
          {(!large || forceEditor) && (
            // 界面优化批（2026-10-16）：spec_json 全文默认收起（开发者反馈「详情页 spec_json 内容不用默认展示」）——
            // 折叠头显行数，展开才懒挂载编辑器；保存/校验链路不变（展开后与原行为一致）
            <Collapse
              size="small"
              style={{ marginTop: 10 }}
              items={[
                {
                  key: 'spec-json',
                  label: `spec_json 全文 · ${specText ? specText.split('\n').length : 0} 行 —— 展开查看 / 编辑`,
                  children: (
                    <JsonEditor
                      value={specText}
                      onChange={setSpecText}
                      readOnly={large}
                      lint={!large}
                      height="480px"
                      placeholder='{ "name": "…", "concepts": [], "relations": [], "instances": [] }'
                    />
                  ),
                },
              ]}
            />
          )}
          {validationErrors.length > 0 && (
            <>
              <Alert type="error" showIcon style={{ marginTop: 12 }} title={`校验未通过（${validationErrors.length} 项）`} />
              <Table<ValidationError>
                rowKey={(r) => `${r.path}::${r.message}`}
                columns={ERR_COLUMNS} scroll={{ x: 'max-content' }}
                dataSource={validationErrors}
                pagination={false}
                size="small"
                style={{ marginTop: 8 }}
              />
            </>
          )}
          {lastVersion != null && validationErrors.length === 0 && <ReloadHintAlert version={lastVersion} />}
        </>
      )}

      {/* REQ-253②（59 号 P2）：缩进大纲→概念层级批量导入（教学场景；仅并入草稿不自动保存） */}
      <Modal
        open={outlineOpen}
        centered
        title="大纲导入（缩进文本 → 概念层级）"
        width={560}
        onCancel={() => setOutlineOpen(false)}
        footer={
          <Space>
            <Button onClick={() => setOutlineOpen(false)}>取消</Button>
            <Button type="primary" disabled={!outlineText.trim()} onClick={applyOutline} data-testid="outline-apply-btn">
              并入草稿（不自动保存）
            </Button>
          </Space>
        }
      >
        <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
          每行一个概念；缩进（tab 或 2 空格）表示子层级，父概念=上一个更浅缩进行。已存在的概念名跳过。仅修改草稿，请检查后手动保存。
        </Typography.Paragraph>
        <Input.TextArea
          rows={10}
          value={outlineText}
          onChange={(e) => setOutlineText(e.target.value)}
          placeholder={'云原生\n  容器编排\n    Kubernetes\n  服务网格\n    Istio'}
          style={{ fontFamily: 'monospace', fontSize: 12 }}
          data-testid="outline-textarea"
        />
        {outlineText.trim() && (
          <div style={{ marginTop: 8 }}>
            {parseOutline(outlineText).slice(0, 8).map((it, i) => (
              <Tag key={i} style={{ margin: 2, marginInlineStart: 8 * it.depth }}>{it.name}</Tag>
            ))}
            {parseOutline(outlineText).length > 8 && <Typography.Text type="secondary" style={{ fontSize: 11 }}>…</Typography.Text>}
          </div>
        )}
      </Modal>
    </>
  )
}

/** REQ-304：大 Spec 摘要卡——不挂 CodeMirror 的轻量查看（体积/计数/头部预览 + 显式打开只读 + 下载）。 */
function LargeSpecSummary({
  ontologyId,
  specText,
  spec,
  onOpen,
}: {
  ontologyId: string
  specText: string
  spec: Spec | null
  onOpen: () => void
}) {
  return (
    <Alert
      type="warning"
      showIcon
      style={{ marginTop: 10 }}
      title={`Spec 体积较大（${(specText.length / 1024).toFixed(0)} KB），已切换为摘要查看`}
      description={
        <Space direction="vertical" size={8} style={{ width: '100%' }}>
          <Typography.Text style={{ fontSize: 12 }}>
            概念 {spec?.concepts?.length ?? 0} · 关系 {spec?.relations?.length ?? 0} · 实例 {spec?.instances?.length ?? 0}
            。全文渲染超大 JSON 易阻塞页面（REQ-304）：对象级编辑请走「图形编辑」，整份备份请下载导出。
          </Typography.Text>
          <pre
            className="large-spec-preview"
            style={{ maxHeight: 180, overflow: 'auto', fontSize: 12, margin: 0, padding: 8, background: 'var(--c-bg-soft)', borderRadius: 6 }}
          >
            {specText.slice(0, 2000)}
            {specText.length > 2000 ? '\n…' : ''}
          </pre>
          <Space>
            <Button size="small" onClick={onOpen} data-testid="large-spec-open-editor">
              仍要打开全文查看（只读，大文档解析需数秒）
            </Button>
            <Button size="small" icon={<DownloadOutlined />} href={api.ontologyExportUrl(ontologyId, 'spec_json')} download>
              下载 spec_json
            </Button>
          </Space>
        </Space>
      }
    />
  )
}

/** REQ-248/G2：CQ 列表编辑（每行一条；空行忽略）。 */
function CqEditor({ value, onChange }: { value: string[]; onChange: (next: string[]) => void }) {
  // 本地镜像 state：prop value 来自父级 spec（仅在 reload 时更新）——若直接渲染 prop，
  // 「添加一条」后 onChange 回写 specText 但 prop 不变，UI 不更新（真机冒烟抓出）。
  const [items, setItems] = useState<string[]>(value)
  const commit = (next: string[]) => {
    setItems(next)
    onChange(next)
  }
  return (
    <div data-testid="cq-editor">
      <Typography.Text type="secondary" style={{ fontSize: 11, display: 'block', marginBottom: 6 }}>
        能力问题=「本体要回答什么问题」（建模方法论第一步，REQ-90）。先列 3~5 条再补概念/关系；随 Spec 保存入库，事后可追溯建模动机。
      </Typography.Text>
      {items.map((q, i) => (
        <Input
          key={i}
          size="small"
          value={q}
          placeholder={`能力问题 ${i + 1}`}
          style={{ marginBottom: 6 }}
          onChange={(e) => {
            const next = [...items]
            next[i] = e.target.value
            commit(next)
          }}
        />
      ))}
      <Button size="small" onClick={() => commit([...items, ''])}>
        添加一条
      </Button>
      {items.length > 0 && (
        <Button size="small" style={{ marginLeft: 8 }} onClick={() => commit(items.slice(0, -1))}>
          移除末条
        </Button>
      )}
    </div>
  )
}

/** REQ-292/M96 编辑保护面板：overlay 清单（人工编辑/新增/删除墓碑）+ 一键恢复机器 base */
function OverlayProtectPanel({ ontologyId, onChanged }: { ontologyId: string; onChanged: () => void }) {
  const { showToast } = useUI()
  const [rows, setRows] = useState<OverlayRow[]>([])
  const [loaded, setLoaded] = useState(false)
  const [busy, setBusy] = useState(false)

  const load = async () => {
    try {
      const r = await api.listOverlays(ontologyId)
      setRows(r.overlays ?? [])
      setLoaded(true)
    } catch {
      setLoaded(true)
    }
  }
  useEffect(() => {
    load()
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [ontologyId])

  const revert = async (row: OverlayRow) => {
    setBusy(true)
    try {
      const r = await api.revertOverlay(ontologyId, row.kind, row.name)
      showToast(`已恢复（v${r.version}）：${r.note}`)
      await load()
      onChanged()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Collapse
      size="small"
      style={{ marginTop: 10 }}
      items={[
        {
          key: 'overlay',
          label: (
            <span data-testid="overlay-panel-label">
              编辑保护（overlay）{rows.length > 0 ? ` · ${rows.length} 项人工编辑` : ' · 无'}
            </span>
          ),
          children:
            rows.length === 0 ? (
              <Typography.Text type="secondary" style={{ fontSize: 12 }}>
                {loaded ? '当前无人工编辑记录。你对概念/关系/实例的修改会被记录；后续「导入合并 / CSV 灌装」等机器重算会自动保留这些人工值（重算不覆盖）。' : '加载中…'}
              </Typography.Text>
            ) : (
              <div>
                <Typography.Paragraph type="secondary" style={{ fontSize: 12, marginBottom: 6 }}>
                  以下对象携带人工编辑：机器重算（导入合并 / CSV 灌装 / 进化采纳）后自动重放保留；「恢复」= 撤回人工编辑回到机器产出值。
                </Typography.Paragraph>
                <Table<OverlayRow>
                  rowKey={(r) => r.logical_id}
                  size="small"
                  pagination={false}
                  dataSource={rows}
                  columns={[
                    { title: '类型', dataIndex: 'kind_label', width: 80 },
                    { title: '对象', dataIndex: 'name', width: 160, ellipsis: true },
                    {
                      title: '动作',
                      dataIndex: 'action',
                      width: 90,
                      render: (v: string) => (
                        <Tag style={{ margin: 0 }} color={{ edit: 'blue', add: 'green', delete: 'red' }[v] ?? 'default'}>
                          {v === 'edit' ? '修改' : v === 'add' ? '新增' : '删除'}
                        </Tag>
                      ),
                    },
                    {
                      title: '状态',
 dataIndex: 'effective',
                      width: 110,
                      render: (_: unknown, r: OverlayRow) =>
                        r.missing ? <Tag style={{ margin: 0 }} color="orange">待重算恢复</Tag> : r.effective ? <Tag style={{ margin: 0 }} color="green">生效中</Tag> : <Tag style={{ margin: 0 }}>未生效</Tag>,
                    },
                    { title: '更新时间', dataIndex: 'updated_at', width: 150, render: (v: string) => (v ?? '').replace('T', ' ').slice(0, 16) },
                    {
                      title: '',
                      width: 80,
                      render: (_: unknown, r: OverlayRow) => (
                        <Button size="small" loading={busy} onClick={() => revert(r)}>
                          恢复
                        </Button>
                      ),
                    },
                  ]}
                />
              </div>
            ),
        },
      ]}
    />
  )
}
