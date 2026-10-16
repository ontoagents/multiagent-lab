import { useState } from 'react'
import OntoChatFlow from './OntoChatFlow'
import KbBuildFlow, { StructuredFlow } from './BuildFromKBFlow'
import OntoExtendFlow from './components/OntoExtendFlow'
import CatalogPane from './components/CatalogPane'
import BuildLayout from './build/BuildLayout'
import BuildPathGrid from './build/BuildPathGrid'
import CustomBuildFlow from './build/CustomBuildFlow'
import KgGuidePage from './build/KgGuidePage'
import { ONTO_BUILD_PATH_KEY, readBuildPath, type BuildPath } from './build/paths'

// ---------------------------------------------------------------------------
// 本体构建（BuildPage，REQ-104 ② → REQ-337/M137 页面化重构）：
//   构建首页收敛为「路径选择页」（BuildPathGrid，六路径卡片降入口+成熟度角标）；
//   custom/ontochat/kb/kg/ontoextend/catalog 各自独立页面，顶栏路径切换器
//   （BuildLayout）常驻切换；路径记忆 eino.onto.buildPath 保留（存量值一次性
//   迁移 'home'，此后显式选择仍记忆——沿 D-O11 迁移先例）。
//   CustomBuildFlow（S1~S4 子页化+草稿暂存）/ KgGuidePage 见 build/ 拆分；
//   未工程化路径显示引导卡、不做空壳交互（D-O11）；oo 双轨已退役（66 号，方案 B）。
//   共享组件 QualityReportCard/SpecGraph/DoneCTA/CatalogPane 复用不动（80 号 U4）。
// ---------------------------------------------------------------------------

export default function BuildPage() {
  const [buildPath, setBuildPath] = useState<BuildPath>(readBuildPath)

  const select = (key: BuildPath) => {
    localStorage.setItem(ONTO_BUILD_PATH_KEY, key)
    setBuildPath(key)
  }

  return (
    <BuildLayout buildPath={buildPath} onSelect={select}>
      {buildPath === 'home' && <BuildPathGrid onSelect={select} />}
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
      {buildPath === 'kb' && (<><KbBuildFlow /><StructuredFlow /></>)}
      {buildPath === 'kg' && <KgGuidePage />}
      {buildPath === 'ontoextend' && <OntoExtendFlow />}
      {buildPath === 'catalog' && <CatalogPane />}
    </BuildLayout>
  )
}
