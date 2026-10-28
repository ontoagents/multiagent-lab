import { useEffect, useState } from 'react'
import { Typography } from 'antd'
import OntoChatFlow from './OntoChatFlow'
import KbBuildFlow, { StructuredFlow } from './BuildFromKBFlow'
import OntoExtendFlow from './components/OntoExtendFlow'
import CatalogPane from './components/CatalogPane'
import BuildLayout from './build/BuildLayout'
import BuildPathGrid from './build/BuildPathGrid'
import CustomBuildFlow from './build/CustomBuildFlow'
import KgGuidePage from './build/KgGuidePage'
import ProductionGuide from './build/ProductionGuide'
import { ONTO_BUILD_PATH_KEY, readBuildPath, type BuildPath } from './build/paths'
import { parseOntoHash, writeOntoHash } from './shared/ontoRoute'

// ---------------------------------------------------------------------------
// 本体构建（BuildPage，REQ-104 ② → REQ-337/M137 页面化重构）：
//   构建首页收敛为「路径选择页」（BuildPathGrid，六路径卡片降入口+成熟度角标）；
//   custom/ontochat/kb/kg/ontoextend/catalog 各自独立页面，顶栏路径切换器
//   （BuildLayout）常驻切换；路径记忆 eino.onto.buildPath 保留（存量值一次性
//   迁移 'home'，此后显式选择仍记忆——沿 D-O11 迁移先例）。
//   CustomBuildFlow（S1~S4 子页化+草稿暂存）/ KgGuidePage 见 build/ 拆分；
//   未工程化路径显示引导卡、不做空壳交互（D-O11）；oo 双轨已退役（66 号，方案 B）。
//   共享组件 QualityReportCard/SpecGraph/DoneCTA/CatalogPane 复用不动（80 号 U4）。
//   REQ-334/M134：首页顶部生产动线引导条（导入/构建→质量门禁→发布→运行消费四步，
//   串质量档案/发布态/消费观测台三段；双模式叙事=动线区分非功能分叉，学习叙事不动）。
// ---------------------------------------------------------------------------

export default function BuildPage() {
  // REQ-470：hash 优先恢复路径（#/onto/build/{path}），无 hash 回落 localStorage 记忆
  const [buildPath, setBuildPath] = useState<BuildPath>(() => {
    const r = parseOntoHash()
    if (r?.col === 'build' && r.path) return r.path as BuildPath
    return readBuildPath()
  })

  // REQ-469：跨入口换路径（命令面板/深链）在构建栏内即时生效——沿 RuntimePage engineKey 同步先例
  useEffect(() => {
    const sync = () => setBuildPath(readBuildPath())
    window.addEventListener('onto-sidebar-change', sync)
    return () => window.removeEventListener('onto-sidebar-change', sync)
  }, [])

  // REQ-470：后退/前进按 hash 恢复路径
  useEffect(() => {
    const onPop = () => {
      const r = parseOntoHash()
      if (r?.col === 'build' && r.path) {
        localStorage.setItem(ONTO_BUILD_PATH_KEY, r.path)
        setBuildPath(r.path as BuildPath)
      }
    }
    window.addEventListener('popstate', onPop)
    window.addEventListener('hashchange', onPop) // REQ-470：同文档 hash 导航同应用
    return () => {
      window.removeEventListener('popstate', onPop)
      window.removeEventListener('hashchange', onPop)
    }
  }, [])

  const select = (key: BuildPath) => {
    localStorage.setItem(ONTO_BUILD_PATH_KEY, key)
    setBuildPath(key)
    // REQ-470：路径切换进历史栈（后退回到先前路径）
    writeOntoHash({ col: 'build', path: key })
  }

  return (
    <BuildLayout buildPath={buildPath} onSelect={select}>
      {buildPath === 'home' && (
        <>
          {/* REQ-484/M283：一主一辅——首屏=新手条+六卡；生产动线（进阶语境）降级到卡片下方折叠条 */}
          <BuildPathGrid onSelect={select} />
          <ProductionGuide onGoBuild={() => select('custom')} />
        </>
      )}
      {buildPath === 'custom' && (
        <CustomBuildFlow
          onGoKbPath={() => select('kb')}
          // REQ-306 A2：AI 创建分流——描述携带进 OntoChat（localStorage 一次性预填）
          onGoOntoChat={(desc) => {
            if (desc) localStorage.setItem('eino.ontochat.preset.desc', desc)
            select('ontochat')
          }}
        />
      )}
      {buildPath === 'ontochat' && <OntoChatFlow onSaved={() => { /* 入库后产物进资产栏；此处留在会话页展示 done 态 */ }} />}
      {buildPath === 'kb' && (
        <>
          <KbBuildFlow />
          {/* REQ-485/M284：双向导视觉分界——结构化映射是「另一种方式」（次级段落，非同流程第二步） */}
          <div style={{ display: 'flex', alignItems: 'baseline', gap: 8, margin: '20px 0 10px', flexWrap: 'wrap' }} data-testid="kb-structured-divider">
            <span style={{ fontSize: 13.5, fontWeight: 600 }}>另一种方式：从结构化数据推导</span>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              有 CSV / JSON 表格数据（非知识库语料）时，直接映射本体骨架
            </Typography.Text>
            <span style={{ flex: 1, borderBottom: '1px dashed var(--c-border, #e6e8f0)', alignSelf: 'center' }} />
          </div>
          <StructuredFlow />
        </>
      )}
      {buildPath === 'kg' && <KgGuidePage />}
      {buildPath === 'ontoextend' && <OntoExtendFlow />}
      {buildPath === 'catalog' && (
        <CatalogPane
          onCreated={(id) => {
            // REQ-405/M201：Catalog onCreated 断头修复——引入成功深链资产详情（沿 DoneCTA 动线）
            localStorage.setItem('eino.onto.sidebar', 'assets')
            localStorage.setItem('eino.onto.asset.focus', id)
            window.dispatchEvent(new CustomEvent('onto-sidebar-change'))
          }}
        />
      )}
    </BuildLayout>
  )
}
