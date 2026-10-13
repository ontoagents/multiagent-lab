// catalogData.ts 权威本体库内置目录（REQ-316/M118）。
// 全部条目经真机验证（2026-10-13 HEAD/GET 200 + 内容嗅探）；扩充随 seeds 轮并同样先验证后收录。
// 许可标注口径：逐条指向来源许可页，平台按 URL 由使用方获取、不二次分发内容。

export interface CatalogEntry {
  id: string
  name: string
  domain: string
  org: string
  /** 规范/文档页 */
  docUrl: string
  /** 机器可读源文件（经真机验证 200 且为 Turtle/RDF-XML） */
  ttlUrl: string
  license: string
  description: string
}

export const ONTOLOGY_CATALOG: CatalogEntry[] = [
  {
    id: 'prov-o',
    name: 'PROV-O（溯源本体）',
    domain: '溯源 / 数据血缘',
    org: 'W3C',
    docUrl: 'https://www.w3.org/TR/prov-o/',
    ttlUrl: 'https://www.w3.org/ns/prov-o',
    license: 'W3C Document License',
    description: 'W3C 溯源本体推荐标准：实体/活动/代理及其因果关系，数据血缘与审计场景的权威词表。',
  },
  {
    id: 'skos',
    name: 'SKOS（简单知识组织系统）',
    domain: '词表 / 分类主题',
    org: 'W3C',
    docUrl: 'https://www.w3.org/TR/skos-reference/',
    ttlUrl: 'https://www.w3.org/2009/08/skos-reference/skos.rdf',
    license: 'W3C Document License',
    description: '受控词表、分类法与主题表建模的 W3C 推荐标准：概念/标签/语义关系，术语体系场景基石。',
  },
  {
    id: 'dcat',
    name: 'DCAT（数据目录词表）',
    domain: '数据集 / 数据目录',
    org: 'W3C',
    docUrl: 'https://www.w3.org/TR/vocab-dcat-3/',
    ttlUrl: 'https://www.w3.org/ns/dcat.ttl',
    license: 'W3C Document License',
    description: '数据集目录与数据服务描述的 W3C 推荐标准（v3）：Dataset/Distribution/DataService。',
  },
  {
    id: 'time',
    name: 'TIME（OWL 时间本体）',
    domain: '时间 / 时序',
    org: 'W3C',
    docUrl: 'https://www.w3.org/TR/owl-time/',
    ttlUrl: 'https://www.w3.org/2006/time',
    license: 'W3C Document License',
    description: '时间区间/时刻/时长与时序关系的 W3C 推荐标准，事件时序建模常用底座。',
  },
  {
    id: 'org',
    name: 'ORG（组织本体）',
    domain: '组织 / 机构',
    org: 'W3C',
    docUrl: 'https://www.w3.org/TR/vocab-org/',
    ttlUrl: 'https://www.w3.org/ns/org.ttl',
    license: 'W3C Document License',
    description: '组织结构/隶属/角色/成员关系词表（W3C 企业本体工作组），组织域建模参照。',
  },
  {
    id: 'ldp',
    name: 'LDP（链接数据平台词表）',
    domain: '链接数据平台',
    org: 'W3C',
    docUrl: 'https://www.w3.org/TR/ldp/',
    ttlUrl: 'https://www.w3.org/ns/ldp.ttl',
    license: 'W3C Document License',
    description: 'W3C Linked Data Platform 推荐标准的资源/容器词表，链接数据架构场景。',
  },
  {
    id: 'ontolex',
    name: 'OntoLex-Lemon（词汇学本体）',
    domain: '语言学 / 词汇语义',
    org: 'W3C',
    docUrl: 'https://www.w3.org/TR/ontolex/',
    ttlUrl: 'https://www.w3.org/ns/lemon/ontolex',
    license: 'W3C Document License',
    description: '词典与词汇语义数据建模的 W3C 社区组规范（OntoLex-Lemon），NLP 术语资源场景。',
  },
  {
    id: 'saref-core',
    name: 'SAREF core（智能应用参考本体）',
    domain: '智能家居 / 能源 / 物联网',
    org: 'ETSI',
    docUrl: 'https://saref.etsi.org/core/',
    ttlUrl: 'https://saref.etsi.org/core/v3.1.1/saref.ttl',
    license: 'ETSI 免费开放使用',
    description: 'ETSI 智能应用参考本体核心（设备/功能/状态/计量），物联网与能源领域工业标准。',
  },
  {
    id: 'dcterms',
    name: 'DCTERMS（都柏林核心元数据术语）',
    domain: '文献元数据',
    org: 'DCMI',
    docUrl: 'https://www.dublincore.org/specifications/dublin-core/dcmi-terms/',
    ttlUrl: 'https://www.dublincore.org/specifications/dublin-core/dcmi-terms/dublin_core_terms.ttl',
    license: 'DCMI（可自由使用）',
    description: '都柏林核心元数据 Initiative 的描述术语集（title/creator/date/subject…），元数据互操作事实标准。',
  },
  {
    id: 'fibo-dev',
    name: 'FIBO Dev（金融行业本体开发版索引）',
    domain: '金融',
    org: 'EDM Council / OMG',
    docUrl: 'https://spec.edmcouncil.org/fibo/',
    ttlUrl: 'https://spec.edmcouncil.org/fibo/ontology/AboutFIBODev/',
    license: 'EDM Council 开放使用',
    description: '金融行业本体标准（EDM Council/OMG）开发版索引入口——大型模块化本体，导入前留意规模。',
  },
]
