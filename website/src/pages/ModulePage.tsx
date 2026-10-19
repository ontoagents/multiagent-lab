import { useMemo } from 'react'
import { CheckCircleFilled, LinkOutlined } from '@ant-design/icons'
import ReactMarkdown from 'react-markdown'
import remarkGfm from 'remark-gfm'
import { MODULES, REPO_URL, moduleText, resolveMdLink, stripFrontmatter, type ModuleInfo } from '../content'
import { useCopy, useLocale } from '../i18n'

// ---------------------------------------------------------------------------
// 模块详细介绍页（REQ-238）：首屏 = 标题 + 速览要点（站点层 brief 数据，直观化）；
// 正文 = platform-knowledge/<模块>/00_导读页.md 构建期内联（单源：内容只在平台知识
// 目录维护，站点随构建自动跟随），下移为"深入了解"区。导读页内的相对 md 互引
// 解析为 GitHub 源文档链接（静态站不落盘全部文档，深入阅读回到仓库）。
// REQ-360：页面 chrome 双语；导读档正文维持中文单源（英文侧诚实标注待译）。
// ---------------------------------------------------------------------------

const COPY = {
  zh: {
    crumbsModules: '功能模块',
    briefAria: '模块速览',
    sourceDocPre: '在 GitHub 查看源文档（platform-knowledge/',
    sourceDocPost: '）',
    detailHead: '深入了解 · 完整导读文档',
    pagerNone: '',
  },
  en: {
    crumbsModules: 'Modules',
    briefAria: 'Module at a glance',
    sourceDocPre: 'View the source document on GitHub (platform-knowledge/',
    sourceDocPost: ')',
    detailHead: 'Deep dive · Full guide document',
    pagerNone: '',
  },
}

/** 英文侧对中文单源导读档的诚实标注（REQ-360） */
const EN_PENDING_NOTE =
  'The full guide below is maintained in Chinese (single source: platform-knowledge/). English translation pending.'

export default function ModulePage({ info }: { info: ModuleInfo }) {
  const { locale } = useLocale()
  const c = useCopy(COPY)
  const mt = moduleText(info, locale)
  const body = useMemo(() => stripFrontmatter(info.md), [info.md])
  const sourceUrl = `${REPO_URL}/blob/main/${info.path}`
  const idx = MODULES.findIndex((m) => m.key === info.key)
  const prev = MODULES[idx - 1]
  const next = MODULES[idx + 1]

  return (
    <main className="page">
      <div className="page-inner">
        <nav className="crumbs">
          <a href="#/module/agents">{c.crumbsModules}</a>
          {' / '}
          <span>{mt.title}</span>
        </nav>
        <h1>{mt.title}</h1>
        <p className="module-tagline">{mt.tagline}</p>

        {/* 速览：30 秒看懂本模块 */}
        <section className="module-brief" aria-label={c.briefAria}>
          <span className="eyebrow">Quick Glance</span>
          <ul>
            {mt.brief.map((b) => (
              <li key={b}>
                <CheckCircleFilled className="brief-check" />
                <span>{b}</span>
              </li>
            ))}
          </ul>
          <div className="module-source">
            <a href={sourceUrl} target="_blank" rel="noreferrer">
              <LinkOutlined /> {c.sourceDocPre}
              {info.path.split('/').pop()}
              {c.sourceDocPost}
            </a>
          </div>
        </section>

        {/* 详细文档（导读页单源全文，下移；英文侧注明中文单源待译） */}
        <div className="module-detail-head">
          {c.detailHead}
          {locale === 'en' && <span className="md-lang-note">{EN_PENDING_NOTE}</span>}
        </div>
        <article className="md-body md-cards-on">
          <ReactMarkdown
            remarkPlugins={[remarkGfm]}
            components={{
              a: ({ href, children }) => {
                const resolved = href ? resolveMdLink(href, info.path) : null
                if (resolved) {
                  return (
                    <a href={resolved} target="_blank" rel="noreferrer">
                      {children}
                    </a>
                  )
                }
                return <a href={href}>{children}</a>
              },
            }}
          >
            {body}
          </ReactMarkdown>
        </article>
        <nav className="module-pager">
          {prev ? (
            <a href={`#/module/${prev.key}`}>← {moduleText(prev, locale).title}</a>
          ) : (
            <span />
          )}
          {next ? <a href={`#/module/${next.key}`}>{moduleText(next, locale).title} →</a> : <span />}
        </nav>
      </div>
    </main>
  )
}
