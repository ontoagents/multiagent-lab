import { useMemo, useState } from 'react'
import { Alert, Button, Checkbox, Input, Modal, Select, Space, Tag, Typography, message } from 'antd'
import { ThunderboltOutlined, CopyOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import { agentFullPayload } from '../api/payload' // REQ-400①③：单源载荷构造器
import type { Agent } from '../api/types'

// REQ-431/M227：混合编排模板（REQ-417 模板机制延伸，参赛叙事落点「平台协调业界智能体干活」）——
// 本地规划者（eino-adk）+ CLI 编码者（kind=cli 连接器）+ 外部审查者（kind=a2a 连接器）三色
// 混合团队一键创建。模板为配置与种子内容，非新编排原语：连接器注册闸门（PATH/握手）如实报，
// 委派审批/审计/沙箱治理沿 REQ-427 已交付面，演示动线标注三色呈现（内部青/CLI volcano/A2A 紫）。

const CLI_BACKENDS = [
  { value: 'claude-code', label: 'Claude Code（claude -p stream-json）' },
  { value: 'opencode', label: 'OpenCode（opencode run）' },
  { value: 'aider', label: 'Aider（aider --message）' },
  { value: 'deepseek-harness', label: 'DeepSeek Harness（dsh headless）' },
]

const PLANNER_INSTRUCTION =
  '你是混合研发团队的规划者（本地智能体，团队协调者）。团队成员：\n' +
  '- 「混合研发·CLI 编码者」：外部 CLI 智能体（业界编码智能体，实现能力强），委派实现类任务；\n' +
  '- 「混合研发·外部审查者」：外部 A2A 智能体（独立对端），委派审查类任务获取独立第三方意见。\n' +
  '职责：把研发任务拆解为 todo；实现任务委派 CLI 编码者（委派描述自包含，附验收标准），审查任务委派外部审查者（附产物清单与要求）；' +
  '汇总双方结果向用户汇报，成员失败时按降级语义改道重试或自行兜底；三类成员（本地/CLI/A2A）各展所长，不要把实现细节自己做除非成员不可用。'

const HYBRID_SEED = {
  key: 'hybrid',
  title: '示例任务：Markdown 待办解析器（本地规划+CLI 实现+外部审查）',
  brief: [
    '【混合编排示例任务】实现一个 Markdown 待办清单解析器，全链路：本地规划 → CLI 编码 → 外部审查：',
    '1) 规划者：拆解 todo（解析器实现/测试/审查三步）；',
    '2) CLI 编码者：在 todo_md/ 下实现 parser.py 与 test_parser.py（解析 - [ ] / - [x] 行为结构化列表，覆盖三种输入），验收=测试全过；',
    '3) 外部审查者：对照任务要求审查实现与测试，给出独立评审意见（通过项/问题清单/改进建议）；',
    '4) 规划者：汇总三方结果，save_file 产出 SUMMARY.md。',
  ].join('\n'),
}

/** 同名智能体存在则改名（-2/-3 后缀），避免重复灌装撞唯一约束 */
function dedupeName(name: string, existing: Agent[]): string {
  if (!existing.some((a) => a.name === name)) return name
  for (let i = 2; i < 99; i++) {
    if (!existing.some((a) => a.name === `${name}-${i}`)) return `${name}-${i}`
  }
  return `${name}-${Date.now()}`
}

export default function HybridTeamTemplate({ agents, onCreated }: { agents: Agent[]; onCreated: (projectId: string) => void }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [backend, setBackend] = useState('claude-code')
  const [cliCwd, setCliCwd] = useState('')
  const [a2aUrl, setA2aUrl] = useState('')
  const [contextInject, setContextInject] = useState<string[]>(['ctx'])
  const [done, setDone] = useState<{ projectId: string } | null>(null)
  const conflicted = useMemo(() => agents.some((a) => a.name === '混合研发·规划者'), [agents])

  const create = async () => {
    if (!a2aUrl.trim()) {
      message.error('外部审查者 A2A 端点必填（可指向另一平台智能体的 /api/a2a/agents/{id}）')
      return
    }
    setBusy(true)
    try {
      const stamp = Date.now().toString(36)
      // 1) 本地规划者
      const plannerName = dedupeName('混合研发·规划者', agents)
      const planner = await api.createAgent({
        name: plannerName,
        description: 'REQ-431 混合编排模板——本地规划者（协调者，eino-adk）',
        instruction: PLANNER_INSTRUCTION,
      })
      // 2) CLI 编码者连接器（注册闸门：PATH 探测真握手，不可达 400 如实报）
      const cliName = `混合研发-CLI编码者-${stamp}`
      const cli = await api.createConnector({
        kind: 'cli',
        name: cliName,
        description: 'REQ-431 混合编排模板——CLI 编码者（业界编码智能体）',
        config: {
          backend,
          ...(cliCwd.trim() ? { cwd: cliCwd.trim() } : {}),
          ...(contextInject.includes('ctx') ? { context_injection: 'auto' } : {}),
        },
      })
      // 3) 外部审查者 A2A 连接器（注册闸门：agent card 真握手）
      const a2aName = `混合研发-外部审查者-${stamp}`
      const a2a = await api.createConnector({
        kind: 'a2a',
        name: a2aName,
        description: 'REQ-431 混合编排模板——外部审查者（独立对端智能体）',
        config: { url: a2aUrl.trim() },
      })
      // 4) 连接器授权给规划者（委派工具进协作面）——PUT 全量替换语义，单源载荷实体基座防清零（REQ-400①③）
      await api.updateAgent(planner.id, agentFullPayload(planner, {
        name: plannerName,
        instruction: PLANNER_INSTRUCTION,
        max_iteration: 25,
        connectors: [cli.id, a2a.id],
      }))
      // 5) 项目（规划者为协调者）
      const proj = await api.createProject({
        name: `混合编排示例-${stamp}`,
        description: 'REQ-431 混合编排模板：本地规划者 + CLI 编码者 + 外部审查者三色协作（平台协调业界智能体）',
        collab_mode: 'agent_as_tool',
        workflow_mode: 'free',
      })
      await api.setProjectAgents(proj.id, [{ agent_id: planner.id, role: 'coordinator' }])
      // 6) 种子任务会话
      await api.createConversation({ scope: 'project', project_id: proj.id, title: HYBRID_SEED.title })
      setDone({ projectId: proj.id })
      message.success('混合编排模板已创建')
    } catch (e: any) {
      message.error(e?.message ?? '模板创建失败（连接器注册闸门如实报——检查 CLI 二进制与 A2A 端点）')
    } finally {
      setBusy(false)
    }
  }

  const finish = () => {
    const projectId = done?.projectId
    setDone(null)
    setOpen(false)
    if (projectId) onCreated(projectId)
  }

  return (
    <>
      <Button
        icon={<ThunderboltOutlined />}
        data-testid="hybrid-team-open"
        onClick={() => setOpen(true)}
      >
        混合编排模板（本地+CLI+外部）
      </Button>
      <Modal
        open={open}
        title={done ? '混合编排模板已创建——演示动线' : '混合编排模板（REQ-431：本地规划者 + CLI 编码者 + 外部审查者）'}
        okText={done ? '打开项目' : '一键创建'}
        okButtonProps={{ loading: busy }}
        onCancel={() => { setOpen(false); setDone(null) }}
        onOk={done ? finish : create}
        width={680}
      >
        {!done ? (
          <Space direction="vertical" style={{ width: '100%' }} size={10}>
            <Typography.Text>
              三色混合团队：<Tag color="cyan" style={{ margin: 0 }}>本地规划者</Tag>
              <Tag color="volcano" style={{ margin: 0 }}>CLI 编码者</Tag>
              <Tag color="purple" style={{ margin: 0 }}>外部审查者</Tag>
              ——平台协调业界智能体干活（委派审批/审计/沙箱治理沿 REQ-427 面生效）。
            </Typography.Text>
            <Typography.Text strong style={{ fontSize: 13 }}>① 本地规划者（自动创建）</Typography.Text>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>任务拆解 + 双向委派：实现任务外委 CLI 编码者，审查任务外委外部审查者（描述即路由语义，能力画像注入）。</Typography.Text>
            <Typography.Text strong style={{ fontSize: 13 }}>② CLI 编码者连接器</Typography.Text>
            <Space.Compact block>
              <Select style={{ width: 280 }} value={backend} onChange={setBackend} options={CLI_BACKENDS} />
              <Input style={{ width: '100%' }} placeholder="编码工作目录 cwd（可选，如项目路径）" value={cliCwd} onChange={(e) => setCliCwd(e.target.value)} />
            </Space.Compact>
            <Typography.Text strong style={{ fontSize: 13 }}>③ 外部审查者连接器（A2A 端点）</Typography.Text>
            <Input placeholder="http://host:8080/api/a2a/agents/{id}（另一平台/智能体的 A2A 端点；本平台亦暴露同款端点可做回环演示）" value={a2aUrl} onChange={(e) => setA2aUrl(e.target.value)} />
            <Checkbox.Group
              value={contextInject}
              onChange={(v) => setContextInject(v as string[])}
              options={[{ label: 'CLI 编码者开启上下文注入（项目文件清单随任务提供给对端——REQ-430）', value: 'ctx' }]}
            />
            {conflicted && (
              <Alert type="info" showIcon message="已存在同名规划者——创建时自动加后缀，不影响使用" />
            )}
            <Alert
              type="warning"
              showIcon
              message="注册闸门如实报"
              description="CLI 后端须已安装并在 PATH（保存时 PATH+--version 真握手）；A2A 端点须可达（agent card 握手）。任一不可达模板创建失败并如实提示——补齐后重试即可。委派外部成员默认受审批档约束（REQ-427）。"
            />
          </Space>
        ) : (
          <Space direction="vertical" style={{ width: '100%' }} size={10}>
            <Typography.Text>
              混合团队就绪：本地规划者（协调者）+ CLI 编码者 + 外部审查者连接器已授权；项目与种子任务会话已建好。
            </Typography.Text>
            <Typography.Text strong style={{ fontSize: 13 }}>演示动线（三色混合编排）</Typography.Text>
            <ol style={{ margin: 0, paddingInlineStart: 20, fontSize: 13, lineHeight: 1.9 }}>
              <li>打开种子任务会话，复制下方任务简报粘贴发送——本地规划者拆解并双向委派；</li>
              <li>委派单按通道分色：🚀 内部委派（青）/ ⌨️ CLI 外部委派（volcano）/ 🌐 A2A 外部委派（紫）；</li>
              <li>「调用轨迹」五泳道看三色过程标记（CLI 成员的 tool_use/text 过程事件实时转译）；</li>
              <li>项目侧板「智能体协作」拓扑：<b>外部 CLI 成员</b>（volcano 边）与<b>外部 A2A 成员</b>（紫边）节点+过程计数；</li>
              <li>治理面：委派外部成员触发审批（若规划者开启 danger/all 档）；CLI 执行可切 docker 沙箱（连接器编辑「执行世界」）；</li>
              <li>能力画像：连接器面展开 CLI/A2A 连接器看技能/在环性/成本档（协调者按画像自动路由）。</li>
            </ol>
            <div>
              <Space>
                <Typography.Text strong style={{ fontSize: 12 }}>{HYBRID_SEED.title}</Typography.Text>
                <Button
                  size="small"
                  icon={<CopyOutlined />}
                  onClick={() => { navigator.clipboard.writeText(HYBRID_SEED.brief).then(() => message.success('任务简报已复制')).catch(() => message.info('复制失败——请手动选择文本复制')) }}
                >
                  复制简报
                </Button>
              </Space>
            </div>
          </Space>
        )}
      </Modal>
    </>
  )
}
