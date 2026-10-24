import { useMemo, useState } from 'react'
import { Alert, Button, Checkbox, Modal, Space, Tag, Typography, message } from 'antd'
import { CopyOutlined, RocketOutlined } from '@ant-design/icons'
import { api } from '../api/client'
import type { Agent } from '../api/types'

// REQ-417/M213：协作研发场景模板与种子（参赛专项②）——规划者/编码者/审查者三角色
// agent_as_tool 编排一键创建（沿 REQ-408 示例灌装先例扩展项目级模板）+两个示例任务种子
// 会话+演示动线指引。模板为配置与种子内容，非新编排原语；run_command 三前置不放宽
//（模板内引导开启：docker 沙箱+绑定目录+审批档位）。

const ROLES = [
  {
    name: '协作研发·规划者',
    desc: '任务拆解 todo → 逐条委派编码者/审查者 → 跟踪汇总，产出 SUMMARY.md',
    instruction:
      '你是协作研发团队（agent_as_tool 模式）的规划者。职责：把用户的研发任务拆解为有序 todo（写入 todo 计划）；逐条委派给「编码者」实现、「审查者」复核，委派任务描述必须自包含；跟踪各成员结果，成员失败时改道重试、分解后再委派或自行兜底；全部完成后用 save_file 把任务拆解与完成状态写入 SUMMARY.md（项目文件，用户可在侧边栏文件视图查看）。',
  },
  {
    name: '协作研发·编码者',
    desc: 'save_file 落盘代码与测试 → 沙箱可用时 run_command 跑测试',
    instruction:
      '你是协作研发团队的编码者。职责：实现代码与测试，用 save_file 落盘到项目目录（建议独立子目录，如 todo_md/）；若运行环境已启用 docker 沙箱且项目绑定目录，用 run_command 执行测试命令验证实现；沙箱不可用时如实说明并以仔细的静态自查代替，不得谎称已运行。只负责实现与验证，不下最终审查结论。',
  },
  {
    name: '协作研发·审查者',
    desc: 'list_files/read_file 阅读产物 → 逐项复核 → 输出 review.md',
    instruction:
      '你是协作研发团队的审查者。职责：用 list_files/read_file 阅读编码者落盘的代码与测试产物，逐项对照任务要求复核；需要定位关键实现时用 grep 工具检索；复核完成后用 save_file 输出 review.md（结论/问题清单/改进建议）。只复核不重写，结论必须引用读到的具体文件内容。',
  },
]

const SEED_TASKS = [
  {
    key: 'a',
    title: '示例任务 A：待办清单解析小工具（实现+测试+审查全链）',
    brief: [
      '【协作研发示例任务 A】为团队实现一个 Markdown 待办清单解析小工具，全链路走规划→编码→执行→审查→产物落盘：',
      '1) 规划者：拆解 todo（实现/测试/审查三步）并逐条委派；',
      '2) 编码者：在项目目录 todo_md/ 下用 save_file 创建 parser.py 与 test_parser.py（解析 - [ ] / - [x] 待办行为结构化列表，测试覆盖未完成/已完成/混合三种输入）；沙箱可用则 run_command 跑 pytest 或 python test_parser.py，不可用则如实说明；',
      '3) 审查者：list_files/read_file 复核代码与测试结果，save_file 输出 todo_md/review.md；',
      '4) 规划者：汇总 save_file 产出 SUMMARY.md（todo 完成状态+产物清单）。',
    ].join('\n'),
  },
  {
    key: 'b',
    title: '示例任务 B：修复嵌套列表解析 bug（复现→修复→回归）',
    brief: [
      '【协作研发示例任务 B】todo_md/parser.py 存在 bug：无序列表的嵌套缩进项被丢弃（输入 "  - 子项" 应作为父项的 sub_items）。演练研发修复流程：',
      '1) 规划者：委派编码者先复现（写复现用例并 run_command 跑，沙箱不可用则静态推演）；',
      '2) 编码者：修复实现并补充回归用例，run_command 回归全部测试；',
      '3) 审查者：复核修复与测试证据，save_file 输出 todo_md/fix_report.md；',
      '4) 规划者：更新 SUMMARY.md。',
    ].join('\n'),
  },
]

/** 同名智能体存在则改名（-2/-3 后缀），避免重复灌装撞唯一约束 */
function dedupeName(name: string, existing: Agent[]): string {
  if (!existing.some((a) => a.name === name)) return name
  for (let i = 2; i < 99; i++) {
    if (!existing.some((a) => a.name === `${name}-${i}`)) return `${name}-${i}`
  }
  return `${name}-${Date.now()}`
}

export default function DevTeamTemplate({ agents, onCreated }: { agents: Agent[]; onCreated: (projectId: string) => void }) {
  const [open, setOpen] = useState(false)
  const [busy, setBusy] = useState(false)
  const [seeds, setSeeds] = useState<string[]>(['a'])
  const [done, setDone] = useState<{ projectId: string; convIds: string[] } | null>(null)

  const conflicted = useMemo(() => ROLES.filter((r) => agents.some((a) => a.name === r.name)).map((r) => r.name), [agents])

  const create = async () => {
    setBusy(true)
    try {
      // 1) 三角色智能体（同名自动后缀）
      const created: Agent[] = []
      for (const r of ROLES) {
        const name = dedupeName(r.name, agents)
        created.push(await api.createAgent({
          name,
          description: `REQ-417 协作研发模板角色——${r.desc}`,
          instruction: r.instruction,
        }))
      }
      const [planner, coder, reviewer] = created
      // 2) 项目（agent_as_tool 自由协作；协调者=规划者）
      const proj = await api.createProject({
        name: '协作研发示例',
        description: 'REQ-417 协作研发模板：规划者→编码者→审查者三角色协作（规划者协调；产物落盘项目文件）',
        collab_mode: 'agent_as_tool',
        workflow_mode: 'free',
      })
      await api.setProjectAgents(proj.id, [
        { agent_id: planner.id, role: 'coordinator' },
        { agent_id: coder.id, role: 'member' },
        { agent_id: reviewer.id, role: 'member' },
      ])
      // 3) 种子任务会话（标题即任务名；简报在完成面板一键复制）
      const convIds: string[] = []
      for (const t of SEED_TASKS.filter((t) => seeds.includes(t.key))) {
        const c = await api.createConversation({ scope: 'project', project_id: proj.id, title: t.title })
        convIds.push(c.id)
      }
      setDone({ projectId: proj.id, convIds })
      message.success('协作研发模板已创建')
    } catch (e: any) {
      message.error(e?.message ?? '模板创建失败')
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
        icon={<RocketOutlined />}
        data-testid="devteam-open"
        onClick={() => setOpen(true)}
      >
        协作研发模板（规划/编码/审查）
      </Button>
      <Modal
        open={open}
        title={done ? '模板已创建——演示动线' : '协作研发项目模板（REQ-417）'}
        okText={done ? '打开项目' : '一键创建'}
        okButtonProps={{ loading: busy }}
        onCancel={() => { setOpen(false); setDone(null) }}
        onOk={done ? finish : create}
        width={640}
      >
        {!done ? (
          <Space direction="vertical" style={{ width: '100%' }} size={10}>
            <Typography.Text>
              一键创建三角色协作团队与示例项目：协调者按 <Tag style={{ margin: 0 }}>agent_as_tool</Tag> 模式委派成员，产物经 save_file 落盘项目文件。
            </Typography.Text>
            {ROLES.map((r) => (
              <div key={r.name}>
                <Typography.Text strong style={{ fontSize: 13 }}>{r.name}</Typography.Text>
                <Typography.Text type="secondary" style={{ fontSize: 12, display: 'block' }}>{r.desc}</Typography.Text>
              </div>
            ))}
            {conflicted.length > 0 && (
              <Alert type="warning" showIcon message={`已存在同名角色（${conflicted.join('、')}）——创建时自动加 -2 后缀，不影响使用`} />
            )}
            <Typography.Text strong style={{ fontSize: 13 }}>示例任务种子（各建一个会话，简报可复制后发送）</Typography.Text>
            <Checkbox.Group
              value={seeds}
              onChange={(v) => setSeeds(v as string[])}
              options={SEED_TASKS.map((t) => ({ label: t.title, value: t.key }))}
            />
            <Alert
              type="info"
              showIcon
              message="run_command 三前置（不放宽，模板内引导）"
              description={
                <span>
                  ①设置页「运行环境」启用 docker 沙箱；②项目侧板「配置」绑定本地目录；③成员执行视图按需开启「工具调用人工审批」。未就绪时协作照常可跑（编码/审查走 save_file/read_file），仅 run_command 降级并如实说明。
                </span>
              }
            />
          </Space>
        ) : (
          <Space direction="vertical" style={{ width: '100%' }} size={10}>
            <Typography.Text>
              项目 <b>协作研发示例</b> 与三角色智能体已就绪；种子任务会话 {done.convIds.length} 个已建好（左侧项目树可见）。
            </Typography.Text>
            <Typography.Text strong style={{ fontSize: 13 }}>演示动线（参赛题目对应能力）</Typography.Text>
            <ol style={{ margin: 0, paddingInlineStart: 20, fontSize: 13, lineHeight: 1.9 }}>
              <li>打开种子任务会话，从下方复制任务简报粘贴发送——协调者自动拆解并委派成员；</li>
              <li>对话头部「过程」面板看 <b>subagent.delegate 委派单</b>（成员/任务）；「调用轨迹」五泳道看时序；</li>
              <li>项目侧板「智能体协作」拓扑回放<b>委派拓扑</b>（成员节点+委派次数）；</li>
              <li>侧板「文件」视图看 save_file 落盘产物（todo_md/、SUMMARY.md）；「Git」视图看产物变更史；</li>
              <li>进阶：接入外部 A2A 智能体（设置页「连接器」新建 kind=A2A）后，协调者可把任务外委派给外部智能体（REQ-416）。</li>
            </ol>
            {SEED_TASKS.filter((t) => seeds.includes(t.key)).map((t) => (
              <div key={t.key}>
                <Space>
                  <Typography.Text strong style={{ fontSize: 12 }}>{t.title}</Typography.Text>
                  <Button
                    size="small"
                    icon={<CopyOutlined />}
                    onClick={() => { navigator.clipboard.writeText(t.brief).then(() => message.success('任务简报已复制')).catch(() => message.info('复制失败——请手动选择文本复制')) }}
                  >
                    复制简报
                  </Button>
                </Space>
              </div>
            ))}
          </Space>
        )}
      </Modal>
    </>
  )
}
