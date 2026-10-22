import { useEffect, useMemo, useState } from 'react'
import {
  Button,
  Divider,
  Dropdown,
  Empty,
  Form,
  Input,
  Modal,
  Popconfirm,
  Popover,
  Result,
  Segmented,
  Select,
  Space,
  Switch,
  Tag,
  Tooltip,
  Typography,
  Upload,
} from 'antd'
import {
  AppstoreOutlined,
  DownOutlined,
  EllipsisOutlined,
  EyeOutlined,
  MinusCircleOutlined,
  PlusOutlined,
  SearchOutlined,
  ThunderboltOutlined,
  UploadOutlined,
} from '@ant-design/icons'
import { api } from '../api/client'
import EmptyGuide from '../components/EmptyGuide'
import type { Agent, Skill, SkillImportReport, ToolInfo } from '../api/types'
import { useUI } from '../store/ui'

type FilterKey = 'all' | 'mounted' | 'unmounted'

/**
 * 技能视图（原型 06 §3.5 / 02 文档 §10）：
 * REQ-326④：筛选自左栏上移为上方「筛选 + 搜索」工具条（三态 Segmented + 计数徽标 +
 * 关键词搜索沿 REQ-233 资产列表搜索先例；预留区收进工具条「更多」Popover），技能左栏退役；
 * 挂载判定不变：挂载 = 技能 id 出现在任一智能体 skills 中。
 * 主体为技能卡片列表 + 新建；编辑弹窗含 instruction / tools 白名单 / resources 文本块 + 注入预览。
 */
export default function SkillsPage() {
  const { showToast, bumpData } = useUI()
  const [skills, setSkills] = useState<Skill[]>([])
  const [tools, setTools] = useState<ToolInfo[]>([])
  const [agents, setAgents] = useState<Agent[]>([])
  const [loadErr, setLoadErr] = useState<string | null>(null)
  const [filter, setFilter] = useState<FilterKey>('all')
  const [q, setQ] = useState('')
  const [modal, setModal] = useState<Skill | 'new' | 'new-collection' | undefined>(undefined)
  const [preview, setPreview] = useState<{ skill: Skill; instruction: string } | null>(null)
  const [importing, setImporting] = useState(false)
  const [importRep, setImportRep] = useState<SkillImportReport | null>(null)

  const reload = () => {
    api.listSkills()
      .then((ss) => {
        setSkills(ss)
        setLoadErr(null)
      })
      .catch((e: any) => {
        setSkills([])
        setLoadErr(e?.message ?? '加载失败')
      })
    // 工具注册表 / 智能体仅用于勾选与挂载计算，失败时优雅降级为空集
    api.listTools().then(setTools).catch(() => setTools([]))
    api.listAgents().then(setAgents).catch(() => setAgents([]))
  }

  useEffect(reload, [])

  // 挂载集合：技能 id 出现在任一智能体 skills 中即为已挂载。
  // REQ-373：挂载集合卡的成员视为「经集合间接挂载」（集合启用时展开注入——与装配口径一致）。
  const mountedIds = useMemo(() => {
    const s = new Set<string>()
    for (const a of agents) {
      for (const id of a.skills ?? []) {
        s.add(id)
        const sk = skills.find((x) => x.id === id)
        if (sk?.kind === 'collection' && sk.enabled) for (const cid of sk.children ?? []) s.add(cid)
      }
    }
    return s
  }, [agents, skills])

  const counts = useMemo(() => {
    const mounted = skills.filter((s) => mountedIds.has(s.id)).length
    return { all: skills.length, mounted, unmounted: skills.length - mounted }
  }, [skills, mountedIds])

  const filtered = useMemo(() => {
    const base =
      filter === 'mounted'
        ? skills.filter((s) => mountedIds.has(s.id))
        : filter === 'unmounted'
          ? skills.filter((s) => !mountedIds.has(s.id))
          : skills
    const ql = q.trim().toLowerCase()
    if (!ql) return base
    return base.filter((s) => [s.name, s.description ?? ''].some((t) => t.toLowerCase().includes(ql)))
  }, [skills, filter, mountedIds, q])

  const openPreview = async (s: Skill) => {
    try {
      const r = await api.skillPreview(s.id)
      setPreview({ skill: s, instruction: r.instruction_block ?? (r as { instruction?: string }).instruction ?? '' })
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  const remove = async (s: Skill) => {
    try {
      await api.deleteSkill(s.id)
      showToast('已删除技能')
      bumpData()
      reload()
    } catch (e: any) {
      showToast(e.message, 'err')
    }
  }

  // REQ-373：zip 导入按包归集（集合随导入创建，成员=本次新建件）——复用导入结果弹窗呈现
  const doZipImport = async (file: File, name: string) => {
    setImporting(true)
    try {
      const rep = await api.importSkills(file, name)
      setImportRep(rep)
      showToast(
        rep.collection
          ? `导入完成：新建 ${rep.created} · 归集「${rep.collection.name}」（${rep.collection.members} 件）`
          : `导入完成：新建 ${rep.created} · 跳过 ${rep.skipped?.length ?? 0} · 失败 ${rep.failed?.length ?? 0}`,
      )
      bumpData()
      reload()
      return true
    } catch (e: any) {
      showToast(e?.message ?? '导入失败', 'err')
      return false
    } finally {
      setImporting(false)
    }
  }

  // REQ-371：目录式技能包导入（zip；同名跳过不覆盖，结果弹窗呈现汇总）
  const doImport = async (options: any) => {
    setImporting(true)
    try {
      const rep = await api.importSkills(options.file as File)
      setImportRep(rep)
      showToast(`导入完成：新建 ${rep.created} · 跳过 ${rep.skipped?.length ?? 0} · 失败 ${rep.failed?.length ?? 0}`)
      bumpData()
      reload()
      options.onSuccess?.(rep)
    } catch (e: any) {
      showToast(e?.message ?? '导入失败', 'err')
      options.onError?.(e)
    } finally {
      setImporting(false)
    }
  }

  return (
    // REQ-326④：技能左栏退役——筛选上移内容区顶部工具条，主体满高（与其他单主区页一致）
    <div className="main">
      <div className="work-main">
        <div className="work-head">
          <div className="work-head-text">
            <div className="work-head-title">
              <Typography.Title level={4} style={{ margin: 0 }}>
                技能
              </Typography.Title>
              <Tag style={{ margin: 0 }}>{filtered.length}</Tag>
            </div>
            <p className="work-head-desc">
              技能是<strong>配置级能力包</strong>（提示词 + 工具白名单），不提供脚本执行环境；装配期按「主指令 + # 启用技能」拼接注入（§6.12）。
            </p>
          </div>
          {/* REQ-371 迭代：导入入口从工具条「更多」弹层上移至头部动作区（与新建并排，高频操作直达） */}
          <Space size={8}>
            <Upload accept=".zip" showUploadList={false} disabled={importing} customRequest={doImport}>
              <Button icon={<UploadOutlined />} loading={importing} data-testid="skills-import-btn">
                导入技能（.zip）
              </Button>
            </Upload>
            {/* REQ-373：新建分裂按钮——主点击直达新建技能；箭头菜单出「新建技能集合」（一卡多子技能） */}
            <Dropdown.Button
              type="primary"
              menu={{
                items: [{ key: 'collection', label: '新建技能集合', icon: <AppstoreOutlined /> }],
                onClick: () => setModal('new-collection'),
              }}
              onClick={() => setModal('new')}
              trigger={['click']}
              icon={<DownOutlined style={{ fontSize: 10 }} />}
            >
              <span data-testid="skills-new-btn">
                <PlusOutlined /> 新建技能
              </span>
            </Dropdown.Button>
          </Space>
        </div>

        {/* REQ-326④：上方筛选 + 搜索组件（三态 + 计数徽标 + 关键词搜索；预留区收进「更多」Popover） */}
        <div className="work-toolbar" data-testid="skills-toolbar">
          <Segmented
            value={filter}
            onChange={(v) => setFilter(v as FilterKey)}
            options={[
              { value: 'all', label: <Space size={6}>全部<Tag style={{ margin: 0 }}>{counts.all}</Tag></Space> },
              { value: 'mounted', label: <Space size={6}>已挂载<Tag color="green" style={{ margin: 0 }}>{counts.mounted}</Tag></Space> },
              { value: 'unmounted', label: <Space size={6}>未挂载<Tag style={{ margin: 0 }}>{counts.unmounted}</Tag></Space> },
            ]}
          />
          <Input
            allowClear
            size="small"
            prefix={<SearchOutlined style={{ color: 'var(--c-ink-3)' }} />}
            placeholder="搜索名称 / 描述"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            style={{ width: 220 }}
            data-testid="skills-search"
          />
          <span style={{ flex: 1 }} />
          <Popover
            trigger="click"
            placement="bottomRight"
            title={
              <Space size={6}>
                预留区
                <Tag color="purple" style={{ margin: 0 }}>
                  规划中
                </Tag>
              </Space>
            }
            content={
              <div style={{ display: 'flex', flexDirection: 'column', gap: 4, minWidth: 140 }}>
                <Button size="small" disabled>技能分组 / 标签</Button>
                <Button size="small" disabled>版本历史</Button>
                {/* 原「技能市场导入」占位保留规划语义；本地 zip 导入已上移头部动作区 */}
                <Button size="small" disabled>技能市场分享</Button>
              </div>
            }
          >
            <Button size="small" icon={<EllipsisOutlined />}>
              更多
            </Button>
          </Popover>
        </div>

          {loadErr ? (
            <div className="work-empty">
              <Result
                status="warning"
                title="技能后端未就绪"
                subTitle={`${loadErr}（M7 后端另行部署）`}
                extra={<Button onClick={reload}>重试</Button>}
              />
            </div>
          ) : filtered.length === 0 ? (
            <div className="work-empty">
              {skills.length === 0 ? (
                <EmptyGuide
                  title="还没有技能"
                  steps={[
                    '点下方「新建技能」：命名并写一段指令（如「严谨回答模式：回答前先复述约束」）',
                    '按需绑定工具白名单与资源链接，保存为可复用的配置级能力包',
                    '到「智能体」配置的「能力」页签勾选挂载，对话时按开关注入',
                  ]}
                  actionLabel="新建第一个技能"
                  onAction={() => setModal('new')}
                  footer="技能与智能体解耦：一份技能可被多个智能体挂载（REQ-120/121）"
                />
              ) : (
                <Empty image={Empty.PRESENTED_IMAGE_SIMPLE} description="该筛选下暂无技能" />
              )}
            </div>
          ) : (
            <div className="skill-grid">
              {filtered.map((s) => {
                const isColl = s.kind === 'collection'
                return (
                <div className="skill-card" key={s.id}>
                  <div className="skill-card-top">
                    <span className="skill-card-icon" style={isColl ? { background: 'var(--c-primary-bg, #f0f5ff)', color: 'var(--c-primary, #4b5cc4)' } : undefined}>
                      {isColl ? <AppstoreOutlined /> : <ThunderboltOutlined />}
                    </span>
                    <span className="skill-card-name" title={s.name}>
                      {s.name}
                    </span>
                    {isColl && (
                      <Tag color="purple" style={{ margin: 0 }} data-testid="skill-kind-tag">
                        集合
                      </Tag>
                    )}
                    {s.builtin && (
                      <Tag color="gold" style={{ margin: 0 }}>
                        内置
                      </Tag>
                    )}
                    <Tag color={s.enabled ? 'blue' : 'default'} style={{ margin: 0 }}>
                      {s.enabled ? '启用' : '停用'}
                    </Tag>
                    {mountedIds.has(s.id) ? (
                      <Tag color="green" style={{ margin: 0 }}>
                        已挂载
                      </Tag>
                    ) : (
                      <Tag style={{ margin: 0 }}>未挂载</Tag>
                    )}
                  </div>
                  <p className="skill-card-desc">
                    {s.description || '未填写描述'}
                    {isColl && (
                      <span style={{ display: 'block', marginTop: 4 }} data-testid="skill-children-count">
                        子技能 {(s.children ?? []).length} 件（挂载后按成员展开注入）
                      </span>
                    )}
                  </p>
                  <div className="skill-card-meta">
                    <span>{isColl ? '工具按成员并集' : `工具 ${s.tools?.length ?? 0}`}</span>
                    <span className="dot">·</span>
                    <span>资源 {s.resources?.length ?? 0}</span>
                  </div>
                  <div className="skill-card-ops">
                    <Button type="link" size="small" icon={<EyeOutlined />} onClick={() => openPreview(s)}>
                      {isColl ? '展开预览' : '注入预览'}
                    </Button>
                    <Button type="link" size="small" onClick={() => setModal(s)}>
                      编辑
                    </Button>
                    {s.builtin ? (
                      <Tooltip title="内置技能不可删除">
                        <span>
                          <Button type="link" size="small" danger disabled>
                            删除
                          </Button>
                        </span>
                      </Tooltip>
                    ) : (
                      <Popconfirm
                        title={`删除${isColl ? '技能集合' : '技能'}「${s.name}」？`}
                        description={isColl ? '只删除集合容器，成员技能保留。' : '已挂载该技能的智能体将失去对应能力。'}
                        okText="删除"
                        okButtonProps={{ danger: true }}
                        cancelText="取消"
                        onConfirm={() => remove(s)}
                      >
                        <Button type="link" size="small" danger>
                          删除
                        </Button>
                      </Popconfirm>
                    )}
                  </div>
                </div>
                )
              })}
            </div>
          )}

        {modal !== undefined && (
          <SkillModal
            skill={modal}
            tools={tools}
            skills={skills}
            onClose={() => setModal(undefined)}
            onSaved={() => {
              setModal(undefined)
              bumpData()
              reload()
            }}
            onPreview={openPreview}
            onZipImport={doZipImport}
          />
        )}
        {preview && <PreviewModal skill={preview.skill} instruction={preview.instruction} onClose={() => setPreview(null)} />}
        {importRep && <ImportResultModal rep={importRep} onClose={() => setImportRep(null)} />}
      </div>
    </div>
  )
}

/** 技能编辑弹窗：普通技能=name/description/instruction（必填）/tools 多选/resources（Form.List）；
 * REQ-373 集合形态=name/description/children 多选（成员须普通技能，不嵌套）/enabled——
 * 新建集合另支持「从技能包（.zip）导入归集」成员来源（集合随导入创建，成员=新建件）。 */
function SkillModal({
  skill,
  tools,
  skills,
  onClose,
  onSaved,
  onPreview,
  onZipImport,
}: {
  skill: Skill | 'new' | 'new-collection'
  tools: ToolInfo[]
  skills: Skill[]
  onClose: () => void
  onSaved: () => void
  onPreview: (s: Skill) => void
  onZipImport: (file: File, name: string) => Promise<boolean>
}) {
  const { showToast } = useUI()
  const [form] = Form.useForm()
  const [busy, setBusy] = useState(false)
  const edit = skill === 'new' || skill === 'new-collection' ? null : skill
  // 集合形态：新建入口「新建技能集合」或编辑集合行
  const isColl = skill === 'new-collection' || edit?.kind === 'collection'
  // 集合成员来源（仅新建集合时可选；编辑集合只勾既有成员）
  const [source, setSource] = useState<'pick' | 'zip'>('pick')
  const [zipFile, setZipFile] = useState<File | null>(null)

  useEffect(() => {
    setSource('pick')
    setZipFile(null)
    if (edit) {
      form.setFieldsValue({
        name: edit.name,
        description: edit.description,
        instruction: edit.instruction,
        tools: edit.tools ?? [],
        resources: edit.resources ?? [],
        children: edit.children ?? [],
        enabled: edit.enabled,
      })
    } else {
      form.setFieldsValue({
        name: '',
        description: '',
        instruction: '',
        tools: [],
        resources: [],
        children: [],
        enabled: true,
      })
    }
  }, [skill, form, edit])

  // 集合成员候选：普通技能（排除集合自身嵌套；编辑时排除自己）
  const memberOptions = skills
    .filter((s) => s.kind !== 'collection' && s.id !== edit?.id)
    .map((s) => ({ value: s.id, label: s.name, title: s.description || s.instruction.slice(0, 60) }))

  const save = async () => {
    let v: any
    try {
      v = await form.validateFields()
    } catch {
      return
    }
    setBusy(true)
    try {
      if (isColl) {
        if (!edit && source === 'zip') {
          // zip 导入归集：集合随导入直接创建（成员=本次新建件），不走普通 create
          if (!zipFile) {
            showToast('请先选择技能包（.zip）', 'err')
            setBusy(false)
            return
          }
          const ok = await onZipImport(zipFile, v.name)
          setBusy(false)
          if (ok) onSaved()
          return
        }
        const payload = {
          name: v.name,
          description: v.description ?? '',
          instruction: '',
          tools: [] as string[],
          resources: [],
          kind: 'collection',
          children: v.children ?? [],
          enabled: typeof v.enabled === 'boolean' ? v.enabled : edit ? edit.enabled : true,
        }
        if (edit) await api.updateSkill(edit.id, payload)
        else await api.createSkill(payload)
        showToast('已保存集合')
        onSaved()
        return
      }
      const resources = ((v.resources ?? []) as { name?: string; content?: string }[])
        .filter((r) => r && (r.name || r.content))
        .map((r) => ({ name: r.name ?? '', content: r.content ?? '' }))
      const payload = {
        name: v.name,
        description: v.description ?? '',
        instruction: v.instruction,
        tools: v.tools ?? [],
        resources,
        // 缺省视为启用：避免后端 full-replace 因字段缺失把技能停用
        enabled: typeof v.enabled === 'boolean' ? v.enabled : edit ? edit.enabled : true,
      }
      if (edit) await api.updateSkill(edit.id, payload)
      else await api.createSkill(payload)
      showToast('已保存')
      onSaved()
    } catch (e: any) {
      showToast(e.message, 'err')
    } finally {
      setBusy(false)
    }
  }

  return (
    <Modal
      open
      centered
      title={
        edit
          ? isColl
            ? `编辑技能集合 · ${edit.name}`
            : `编辑技能 · ${edit.name}`
          : isColl
            ? '新建技能集合'
            : '新建技能'
      }
      width={680}
      onCancel={onClose}
      styles={{ body: { maxHeight: 'calc(100vh - 220px)', overflowY: 'auto', paddingRight: 8 } }}
      footer={
        <Space style={{ display: 'flex', justifyContent: 'space-between', width: '100%' }}>
          <Button disabled={!edit} onClick={() => edit && onPreview(edit)}>
            {isColl ? '展开预览' : '注入预览'}
          </Button>
          <Space>
            <Button onClick={onClose}>取消</Button>
            <Button type="primary" loading={busy} onClick={save}>
              保存
            </Button>
          </Space>
        </Space>
      }
    >
      <Form form={form} layout="vertical" requiredMark={false}>
        <Form.Item name="name" label="名称" rules={[{ required: true, message: '名称必填' }]}>
          <Input placeholder={isColl ? '如：科学计算技能集' : '如：K8s 排障指引'} maxLength={60} />
        </Form.Item>
        <Form.Item name="description" label="描述">
          <Input.TextArea autoSize={{ minRows: 2, maxRows: 3 }} placeholder={isColl ? '集合用途（成员各自描述仍参与触发）' : '用途与触发场景（供智能体理解何时启用）'} />
        </Form.Item>
        <Form.Item
          name="enabled"
          label="启用"
          valuePropName="checked"
          extra={isColl ? '停用集合=成员整体不展开注入；成员自身的启停独立生效。' : '停用后不参与装配注入；保存时后端按 full-replace 处理，缺省视为停用。'}
        >
          <Switch />
        </Form.Item>
        {isColl ? (
          <>
            {!edit && (
              <Form.Item label="成员来源" style={{ marginBottom: 8 }}>
                <Segmented
                  value={source}
                  onChange={(v) => setSource(v as 'pick' | 'zip')}
                  options={[
                    { value: 'pick', label: '勾选既有技能' },
                    { value: 'zip', label: '从技能包（.zip）导入归集' },
                  ]}
                />
              </Form.Item>
            )}
            {source === 'pick' || edit ? (
              <Form.Item
                name="children"
                label="集合成员"
                extra={
                  memberOptions.length === 0
                    ? '暂无可选普通技能（先创建技能，或改用 zip 导入归集）。'
                    : '成员须为普通技能（集合不嵌套）；挂载集合=启用其下全部启用成员，工具白名单取并集。'
                }
              >
                <Select
                  mode="multiple"
                  allowClear
                  showSearch
                  optionFilterProp="label"
                  placeholder="选择收进集合的技能"
                  options={memberOptions}
                />
              </Form.Item>
            ) : (
              <Form.Item label="技能包（.zip）" required extra="解析 SKILL.md 标准包并归集：新建件收进本集合；同名跳过与失败件不入集（结果弹窗明细）。">
                <Upload
                  accept=".zip"
                  maxCount={1}
                  beforeUpload={(f) => {
                    setZipFile(f as File)
                    return false
                  }}
                  onRemove={() => setZipFile(null)}
                  fileList={zipFile ? [{ uid: '-1', name: zipFile.name, status: 'done' }] : []}
                >
                  <Button icon={<UploadOutlined />}>选择 zip 技能包</Button>
                </Upload>
              </Form.Item>
            )}
          </>
        ) : (
          <>
            <Form.Item
              name="instruction"
              label="指令（instruction）"
              rules={[{ required: true, message: '指令必填' }]}
              extra="装配期按「主指令 + # 启用技能」拼接注入；技能仅配置级注入，不提供脚本执行环境。"
            >
              <Input.TextArea autoSize={{ minRows: 6, maxRows: 16 }} placeholder="技能提示词…" />
            </Form.Item>
            <Form.Item
              name="tools"
              label="工具白名单"
              extra={tools.length === 0 ? '工具注册表为空；请确认 M5 后端已就绪。' : '来自工具注册表；与智能体自身工具合并去重（§6.12）。'}
            >
              <Select
                mode="multiple"
                allowClear
                showSearch
                optionFilterProp="label"
                placeholder="选择该技能可用的工具"
                options={tools.map((t) => ({ value: t.id, label: `${t.name}${t.source ? ` · ${t.source}` : ''}` }))}
              />
            </Form.Item>
            <Form.Item label="资源（resources，可选）" style={{ marginBottom: 0 }} extra="按需拼入上下文（P2）；内容较大时后续演进为 get_skill_resource 工具。">
              <Form.List name="resources">
                {(fields, { add, remove }) => (
                  <div className="res-list">
                    {fields.map((field) => (
                      <div className="res-row" key={field.key}>
                        <Form.Item name={[field.name, 'name']} noStyle>
                          <Input placeholder="资源名" style={{ width: 180 }} />
                        </Form.Item>
                        <Form.Item name={[field.name, 'content']} noStyle>
                          <Input.TextArea autoSize={{ minRows: 1, maxRows: 4 }} placeholder="资源内容" style={{ flex: 1 }} />
                        </Form.Item>
                        <Button type="text" danger icon={<MinusCircleOutlined />} onClick={() => remove(field.name)} />
                      </div>
                    ))}
                    <Button type="dashed" block icon={<PlusOutlined />} onClick={() => add({ name: '', content: '' })}>
                      添加资源
                    </Button>
                  </div>
                )}
              </Form.List>
            </Form.Item>
          </>
        )}
        {isColl && (
          <Divider plain style={{ margin: '4px 0 0' }}>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              集合是容器：不注入自身指令，装配期按启用成员展开目录（REQ-373）
            </Typography.Text>
          </Divider>
        )}
      </Form>
    </Modal>
  )
}

/** 注入预览：展示后端组合后的完整 instruction（§6.12：主指令 + # 启用技能 段）。
 * REQ-373 集合预览=按启用成员展开的目录（与装配期目录形态同构）。 */
function PreviewModal({ skill, instruction, onClose }: { skill: Skill; instruction: string; onClose: () => void }) {
  const isColl = skill.kind === 'collection'
  return (
    <Modal open centered title={`注入预览 · ${skill.name}`} width={680} onCancel={onClose} footer={<Button onClick={onClose}>关闭</Button>}>
      <Typography.Paragraph type="secondary" style={{ fontSize: 12 }}>
        {isColl ? (
          <>技能集合按启用成员展开为目录注入：每个成员独立一行（名称+描述），正文经 <Typography.Text code>load_skill</Typography.Text> 懒加载（REQ-373）。</>
        ) : (
          <>装配期将该技能拼入智能体主指令：主指令 + <Typography.Text code># 启用技能</Typography.Text> +{' '}
          <Typography.Text code>{'<skill name="…">…</skill>'}</Typography.Text>（§6.12）。</>
        )}
      </Typography.Paragraph>
      <pre className="preview-pre">{instruction || '（空——集合停用或无启用成员）'}</pre>
    </Modal>
  )
}

/** REQ-371 导入结果：计数徽标 + 跳过/失败明细（人话原因，诚实呈现；新建名单不逐条铺陈） */
function ImportResultModal({ rep, onClose }: { rep: SkillImportReport; onClose: () => void }) {
  const skipped = rep.skipped ?? []
  const failed = rep.failed ?? []
  return (
    <Modal
      open
      centered
      title="导入技能结果"
      width={560}
      onCancel={onClose}
      footer={
        <Button type="primary" onClick={onClose} data-testid="import-done">
          完成
        </Button>
      }
    >
      <Space direction="vertical" style={{ width: '100%' }} size={10}>
        <Space size={8} wrap>
          <Tag color="green" style={{ margin: 0 }} data-testid="import-created">
            新建 {rep.created}
          </Tag>
          <Tag color="orange" style={{ margin: 0 }} data-testid="import-skipped">
            跳过 {skipped.length}
          </Tag>
          <Tag color={failed.length ? 'red' : 'default'} style={{ margin: 0 }} data-testid="import-failed">
            失败 {failed.length}
          </Tag>
          {rep.lists_shrunk && <Tag color="purple">明细超限截断（计数完整）</Tag>}
          {rep.collection && (
            <Tag color="purple" style={{ margin: 0 }} data-testid="import-collection">
              归集「{rep.collection.name}」· {rep.collection.members} 件
            </Tag>
          )}
        </Space>
        {skipped.length > 0 && (
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              跳过明细：同名已存在不覆盖 / 包内重名
            </Typography.Text>
            <div className="import-detail-list" data-testid="import-skip-list">
              {skipped.map((s, i) => (
                <div key={i} style={{ fontSize: 12 }}>
                  <Typography.Text code>{s.name}</Typography.Text> — {s.reason}
                </div>
              ))}
            </div>
          </div>
        )}
        {failed.length > 0 && (
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              失败明细
            </Typography.Text>
            <div className="import-detail-list" data-testid="import-fail-list">
              {failed.map((f, i) => (
                <div key={i} style={{ fontSize: 12, color: 'var(--c-danger, #cf1322)' }}>
                  <Typography.Text code>{f.name}</Typography.Text> — {f.reason}
                </div>
              ))}
            </div>
          </div>
        )}
        {(rep.notes ?? []).length > 0 && (
          <div>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              导入标注：截断 / 资源超限跳过（诚实呈现）
            </Typography.Text>
            <div className="import-detail-list" data-testid="import-notes-list">
              {(rep.notes ?? []).map((n, i) => (
                <div key={i} style={{ fontSize: 12 }}>
                  <Typography.Text code>{n.skill}</Typography.Text> — {n.notes.join('；')}
                </div>
              ))}
            </div>
          </div>
        )}
      </Space>
    </Modal>
  )
}
