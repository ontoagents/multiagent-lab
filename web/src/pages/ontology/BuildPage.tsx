import { useEffect, useState } from 'react'
import { Typography } from 'antd'
import OntoChatFlow from './OntoChatFlow'
import KbBuildFlow, { StructuredFlow } from './BuildFromKBFlow'
import OntoExtendFlow from './components/OntoExtendFlow'
import CatalogPane from './components/CatalogPane'
import ConceptParkingFlow from './build/ConceptParkingFlow'
import CustomBuildFlow from './build/CustomBuildFlow'
import DataSourceBuildFlow from './build/DataSourceBuildFlow'
import BuildHub from './build/BuildHub'
import PathShell from './build/PathShell'
import { ONTO_BUILD_PATH_KEY, readBuildPath, type BuildPath } from './build/paths'
import { deepLinkOnto, parseOntoHash, writeOntoHash } from './shared/ontoRoute'

// ---------------------------------------------------------------------------
// 本体构建（BuildPage）：REQ-598/M409 Hub+独占路径页（111 号 §3.1~§3.5 结构性收敛）。
//   路径选择、路径宿主、跨栏跳板三种角色分离：# /onto/build（无 path 段）=Hub 轻量
//   入口页；#/onto/build/{path}=路径独占页（组件原样包 PathShell 壳，页内不陈列其他
//   路径入口）；#/onto/build/home 保留为 hub 别名（进页 replace 成无 path 形态）；
//   #/onto/build/kg 退役为重定向（等效旧 KgGuidePage 行为，旧深链不断链）。
//   路由写入约定：页级跳转 push、页内细化 replace；hash 内 query 参数为跨路径一次性
//   交接通道（?onto=&step=&source=&desc=，消费即清——记忆与交接分治，customOnto/
//   customStep/s1tab 记忆职责保留）。
//   路径记忆 eino.onto.buildPath=「最后访问的路径页」（hub 不写此键）；REQ-337 一次性
//   迁移退役（hub 成为缺省）。顶栏 8 页签切换行（旧 BuildLayout）与首页路径卡
//   （旧 BuildPathGrid）随本方案退役；生产动线迁 Hub 区块 C；命令面板 Mod+K 保留为
//   全局路径切换辅助。共享壳 PathShell/FlowSteps/ConvergeCard 复用平台布局语言。
// ---------------------------------------------------------------------------

/** kg 重定向页（#/onto/build/kg）：等效旧 KgGuidePage——深链消费与审计栏（Q3：直跳，
 *  断链风险最低）。KG 图谱抽取与消费动线由第五栏承载（D-O19/111 号 §3.3-⑧）。 */
function KgRedirect() {
  useEffect(() => {
    deepLinkOnto({ col: 'audit' })
  }, [])
  return (
    <div className="work-main">
      <Typography.Text type="secondary" style={{ fontSize: 12.5 }} data-testid="kg-redirect-note">
        正在前往「消费与审计」栏——KG 图谱浏览 / GraphRAG 试查 / 决策审计由该栏承载…
      </Typography.Text>
    </div>
  )
}

export default function BuildPage() {
  // hash 优先：有 path 段→路径页；无 path/home→Hub；hash 非本栏（legacy 直写键入口）→记忆键。
  // 'kg' 仅来自 hash 解析（退役重定向），navigate 永不写入。
  const [buildPath, setBuildPath] = useState<BuildPath | 'kg'>(() => {
    const r = parseOntoHash()
    if (r?.col === 'build') return r.path && r.path !== 'home' ? (r.path as BuildPath | 'kg') : 'home'
    return readBuildPath()
  })
  // REQ-508/M316：概念停车场弹窗（P1 仍为弹窗动线；REQ-600 页面化退役）——Hub「有学习材料」入口
  const [materialOpen, setMaterialOpen] = useState(false)

  // REQ-598：#/onto/build/home 别名进页 replace 成无 path 形态（旧深链/书签不断）
  useEffect(() => {
    const r = parseOntoHash()
    if (r?.col === 'build' && r.path === 'home') writeOntoHash({ col: 'build' }, true)
  }, [])

  // 统一 hash 应用：popstate/hashchange（后退前进）+ onto-sidebar-change（跨入口换路径）
  // 共用——hub 无 path 分支在此补齐（旧行为仅认有 path 的 hash，后退回 hub 被静默忽略）。
  // hash 非本栏时回落记忆键（legacy 直写键调用方：资产「演化」/学习页「援引到构建」不写 hash）。
  const applyRoute = () => {
    const r = parseOntoHash()
    if (r?.col === 'build') {
      if (!r.path || r.path === 'home') {
        if (r.path === 'home') writeOntoHash({ col: 'build' }, true)
        setBuildPath('home') // hub 不写记忆键
        return
      }
      if (r.path !== 'kg') localStorage.setItem(ONTO_BUILD_PATH_KEY, r.path) // kg 退役不占记忆
      setBuildPath(r.path as BuildPath | 'kg')
      return
    }
    setBuildPath(readBuildPath())
  }
  useEffect(() => {
    window.addEventListener('onto-sidebar-change', applyRoute)
    window.addEventListener('popstate', applyRoute)
    window.addEventListener('hashchange', applyRoute) // REQ-470：同文档 hash 导航同应用
    return () => {
      window.removeEventListener('onto-sidebar-change', applyRoute)
      window.removeEventListener('popstate', applyRoute)
      window.removeEventListener('hashchange', applyRoute)
    }
  }, [])

  const navigate = (key: BuildPath, params?: Record<string, string>) => {
    if (key === 'home') {
      setBuildPath('home')
      writeOntoHash({ col: 'build' }) // 页级跳转进历史栈（后退回先前路径页）
      return
    }
    localStorage.setItem(ONTO_BUILD_PATH_KEY, key)
    setBuildPath(key)
    writeOntoHash({ col: 'build', path: key, params })
  }

  return (
    <>
      {buildPath === 'home' && <BuildHub onGo={navigate} onMaterialStart={() => setMaterialOpen(true)} />}
      {buildPath === 'custom' && (
        <PathShell path="custom" onBack={() => navigate('home')}>
          <CustomBuildFlow
            onGoKbPath={() => navigate('kb')}
            // REQ-306 A2：AI 创建分流——描述携带进 OntoChat（REQ-598 改路由参数 ?desc=
            // 一次性预填；P2-a 收口退役 ontochat.preset.desc 旧键回退读）
            onGoOntoChat={(desc) => navigate('ontochat', desc ? { desc } : undefined)}
          />
        </PathShell>
      )}
      {buildPath === 'ontochat' && (
        <PathShell path="ontochat" onBack={() => navigate('home')}>
          <OntoChatFlow onSaved={() => { /* 入库后产物进资产栏；此处留在会话页展示 done 态 */ }} />
        </PathShell>
      )}
      {buildPath === 'kb' && (
        <PathShell path="kb" onBack={() => navigate('home')}>
          <KbBuildFlow />
          {/* REQ-485/M284：双向导视觉分界——结构化映射是「另一种方式」（次级段落）；
              REQ-600 拆独立路由 #/onto/build/structured，本页过渡态保持双段 */}
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, margin: '20px 0 10px', flexWrap: 'wrap' }} data-testid="kb-structured-divider">
            <span style={{ fontSize: 13.5, fontWeight: 600 }}>另一种方式：从结构化数据推导</span>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              有 CSV / JSON 表格数据（非知识库语料）时，直接映射本体骨架
            </Typography.Text>
            <span style={{ flex: 1, borderBottom: '1px dashed var(--c-border, #e6e8f0)', alignSelf: 'center' }} />
          </div>
          <StructuredFlow />
        </PathShell>
      )}
      {buildPath === 'kg' && <KgRedirect />}
      {buildPath === 'datasource' && (
        <PathShell path="datasource" onBack={() => navigate('home')}>
          <DataSourceBuildFlow
            onGoCustom={(id) => {
              // REQ-595：提案落库后深链 custom ② 编辑——REQ-598 交接走路由参数
              //（?onto=&step= 一次性，消费即清；customOnto/customStep 记忆键职责保留）
              navigate('custom', { onto: id, step: '1' })
            }}
          />
        </PathShell>
      )}
      {buildPath === 'ontoextend' && (
        <PathShell path="ontoextend" onBack={() => navigate('home')}>
          <OntoExtendFlow />
        </PathShell>
      )}
      {buildPath === 'catalog' && (
        <PathShell path="catalog" onBack={() => navigate('home')}>
          <CatalogPane
            onCreated={(id) => {
              // REQ-405/M201：引入成功深链资产详情（gotoPane 收敛 deepLinkOnto 单点）
              deepLinkOnto({ col: 'assets', assetId: id }, [['eino.onto.asset.focus', id]])
            }}
          />
        </PathShell>
      )}
      <ConceptParkingFlow
        open={materialOpen}
        onClose={() => setMaterialOpen(false)}
        onCreated={(id) => {
          // REQ-508/M316：材料「一键成骨架」落点 custom ② 编辑——交接走路由参数
          //（?onto=&step= 一次性；沿 datasource onGoCustom 同参先例）
          navigate('custom', { onto: id, step: '1' })
        }}
      />
    </>
  )
}
