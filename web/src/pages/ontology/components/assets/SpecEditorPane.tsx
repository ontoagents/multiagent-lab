import { useEffect, useState } from 'react'
import { Alert, Button, Collapse, Form, Input, Table, Typography } from 'antd'
import { ReloadOutlined } from '@ant-design/icons'
import { api, ApiError } from '../../../../api/client'
import type { Ontology, Spec, ValidationError } from '../../../../api/types'
import { useUI } from '../../../../store/ui'
import { ERR_COLUMNS, ReloadHintAlert } from '../../shared'
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
}: {
  ontology: Ontology
  spec: Spec | null
  specLoading: boolean
  specErr: string | null
  onReloadSpec: () => void
  onMetaSaved: () => void
  onSpecSaved: (version: number) => void
}) {
  const { showToast } = useUI()
  const [metaForm] = Form.useForm()
  const [specText, setSpecText] = useState('')
  const [savingMeta, setSavingMeta] = useState(false)
  const [savingSpec, setSavingSpec] = useState(false)
  const [validationErrors, setValidationErrors] = useState<ValidationError[]>([])
  const [lastVersion, setLastVersion] = useState<number | null>(null)

  useEffect(() => {
    metaForm.setFieldsValue({ name: ontology.name, description: ontology.description ?? '' })
  }, [ontology.id, ontology.name, ontology.description, metaForm])

  useEffect(() => {
    setSpecText(spec ? JSON.stringify(spec, null, 2) : '')
    setValidationErrors([])
  }, [spec])

  const large = specText.length > 200_000

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
      const r = await api.saveSpec(ontology.id, parsed)
      showToast(`Spec 已保存（version ${r.version}）`)
      setLastVersion(r.version)
      onSpecSaved(r.version)
    } catch (e: any) {
      if (e instanceof ApiError && e.validationErrors?.length) {
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
        <span className="hit-spacer" />
        <Typography.Text type="secondary" style={{ fontSize: 12 }}>
          {specLoading
            ? '加载中…'
            : spec
              ? `概念 ${spec.concepts?.length ?? 0} · 关系 ${spec.relations?.length ?? 0} · 实例 ${spec.instances?.length ?? 0}`
              : '尚未保存过 Spec'}
        </Typography.Text>
        <Button size="small" icon={<ReloadOutlined />} onClick={onReloadSpec} disabled={specLoading}>
          重新加载
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
          {large && (
            <Alert
              type="warning"
              showIcon
              style={{ margin: '10px 0' }}
              title="Spec 体积较大，已切换为只读"
              description="请在本地编辑后经导入 / 导出接口处理，避免浏览器卡顿。"
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
          <div style={{ marginTop: 10 }}>
            <JsonEditor
              value={specText}
              onChange={setSpecText}
              readOnly={large}
              height="480px"
              placeholder='{ "name": "…", "concepts": [], "relations": [], "instances": [] }'
            />
          </div>
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
    </>
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
