// REQ-452/M248 UI 冒烟：模型调用重试与故障转移——备援模型连接产品面。
// A 模型页「备援模型连接」多选在位 / B 选择备援保存 → API 持久化 / C 重开回显 /
// D 主连接自身不出现在备援选项 / E 清空保存回落空数组 / 零 pageerror。
// 运行：node smoke/req452/ui.mjs（本地栈 :8080；无需模型凭证——仅配置面）。
import { chromium } from 'playwright-core'
import fs from 'node:fs'

const BASE = 'http://127.0.0.1:8080'
const SHOT_DIR = new URL('.', import.meta.url).pathname
const exe = process.env.HOME + '/.cache/ms-playwright/chromium-1148/chrome-linux/chrome'
const LD = '/tmp/alsa/ext/usr/lib/x86_64-linux-gnu'

let pass = 0, fail = 0
const ok = (cond, label) => { if (cond) { pass++; console.log(`  ✓ ${label}`) } else { fail++; console.log(`  ✗ FAIL: ${label}`) } }
const sleep = (ms) => new Promise((r) => setTimeout(r, ms))

const api = async (path, method = 'GET', body) => {
  const res = await fetch(BASE + path, { method, headers: { 'Content-Type': 'application/json' }, body: body ? JSON.stringify(body) : undefined })
  return { status: res.status, data: await res.json().catch(() => null) }
}

const browser = await chromium.launch({
  executablePath: exe, headless: true,
  env: { ...process.env, LD_LIBRARY_PATH: LD },
  args: ['--no-sandbox', '--disable-dev-shm-usage', '--font-render-hinting=none'],
})
const page = await browser.newPage({ viewport: { width: 1600, height: 900 } })
const pageErrors = []
page.on('pageerror', (e) => pageErrors.push(String(e)))

const suffix = String(Date.now()).slice(-6)
const created = { conns: [], agent: null }
try {
  // ---------- 夹具：两连接一智能体 ----------
  const c1 = await api('/api/model-connections', 'POST', { name: `smoke452-main-${suffix}`, conn_type: 'chat', protocol: 'openai_compat', base_url: 'http://127.0.0.1:1', model_name: 'm0', enabled: true, api_key: '' })
  const c2 = await api('/api/model-connections', 'POST', { name: `smoke452-backup-${suffix}`, conn_type: 'chat', protocol: 'openai_compat', base_url: 'http://127.0.0.1:2', model_name: 'm1', enabled: true, api_key: '' })
  created.conns = [c1.data?.id, c2.data?.id].filter(Boolean)
  const ag = await api('/api/agents', 'POST', { name: `smoke452-agent-${suffix}`, description: '冒烟夹具', instruction: 'i', model_conn_id: c1.data?.id })
  created.agent = ag.data?.id

  // ---------- A 打开智能体侧板模型页 ----------
  console.log('A. 侧板「模型与参数」页备援字段在位')
  await page.goto(BASE + '/', { waitUntil: 'networkidle' })
  await page.evaluate(() => localStorage.clear())
  await page.goto(BASE + '/', { waitUntil: 'networkidle' })
  await page.click('.gn-item:has-text("智能体")')
  await sleep(600)
  await page.click(`.sidebar-panel :text("smoke452-agent-${suffix}")`)
  await sleep(600)
  await page.click('button[aria-label="模型"]')
  await sleep(600)
  const failoverLabel = await page.locator('.proj-panel :text("备援模型连接")').count()
  ok(failoverLabel > 0, 'A1 「备援模型连接」字段在模型页渲染')
  // tooltip 仅悬停渲染：悬停字段问号图标断言诚实标注语义
  const tipIcon = page.locator('.proj-panel .ant-form-item:has-text("备援模型连接") .ant-form-item-tooltip').first()
  await tipIcon.hover().catch(() => {})
  await sleep(500)
  const failoverTip = await page.locator('.ant-tooltip:has-text("run.warning")').count()
  ok(failoverTip > 0, 'A2 字段说明含 run.warning 留痕语义（悬停 tooltip）')
  await page.screenshot({ path: SHOT_DIR + '01-failover-field.png' })

  // ---------- B 选择备援连接保存 → API 持久化 ----------
  console.log('B. 选择备援连接保存 → 持久化')
  const sel = page.locator('.proj-panel .ant-select').filter({ hasText: '留空 = 不启用故障转移' }).first()
  const selCount = await sel.count()
  ok(selCount > 0, 'B1 备援多选占位在位（留空=不启用）')
  await sel.click()
  await sleep(400)
  // D 主连接不出现在备援选项（当前主连接已被排除）
  const optMain = await page.locator(`.ant-select-dropdown .ant-select-item:has-text("smoke452-main-${suffix}")`).count()
  ok(optMain === 0, 'D1 当前主连接不在备援选项中')
  const optBackup = await page.locator(`.ant-select-dropdown .ant-select-item:has-text("smoke452-backup-${suffix}")`).count()
  ok(optBackup > 0, 'D2 备援连接出现在选项中')
  await page.click(`.ant-select-dropdown .ant-select-item:has-text("smoke452-backup-${suffix}")`)
  await sleep(300)
  await page.keyboard.press('Escape')
  const saveBtn = page.locator('.proj-view-actions button.ant-btn-primary').first()
  await saveBtn.click()
  await sleep(800)
  const after = await api(`/api/agents/${created.agent}`)
  ok(Array.isArray(after.data?.model_failover_conn_ids) && after.data.model_failover_conn_ids.includes(c2.data?.id), 'B2 保存后 API 回读含备援连接 id')

  // ---------- C 重开回显 ----------
  console.log('C. 重开侧板回显')
  await page.reload({ waitUntil: 'networkidle' })
  await sleep(600)
  await page.click('.gn-item:has-text("智能体")')
  await sleep(400)
  await page.click(`.sidebar-panel :text("smoke452-agent-${suffix}")`)
  await sleep(500)
  await page.click('button[aria-label="模型"]')
  await sleep(600)
  const echoed = await page.locator(`.proj-panel .ant-select:has-text("smoke452-backup-${suffix}")`).count()
  ok(echoed > 0, 'C1 重开后备援选择回显')
  await page.screenshot({ path: SHOT_DIR + '02-echo.png' })

  // ---------- E 清空保存 ----------
  console.log('E. 清空保存回落空数组')
  const clearIcon = page.locator('.proj-panel .ant-form-item:has-text("备援模型连接") .ant-select-clear').first()
  await clearIcon.click()
  await sleep(300)
  await page.locator('.proj-view-actions button.ant-btn-primary').first().click()
  await sleep(800)
  const cleared = await api(`/api/agents/${created.agent}`)
  ok(Array.isArray(cleared.data?.model_failover_conn_ids) && cleared.data.model_failover_conn_ids.length === 0, 'E1 清空保存后 API 回读为空数组')
} catch (e) {
  fail++
  console.log('  ✗ EXCEPTION:', String(e).slice(0, 300))
  await page.screenshot({ path: SHOT_DIR + '99-error.png' }).catch(() => {})
} finally {
  // teardown
  if (created.agent) await api(`/api/agents/${created.agent}`, 'DELETE')
  for (const id of created.conns) await api(`/api/model-connections/${id}`, 'DELETE')
  const before = JSON.stringify(await (await fetch(BASE + '/api/agents')).json()).length
  ok(pageErrors.length === 0, `零 pageerror（实际 ${pageErrors.length}）`)
  console.log(`\n结果: ${pass} 通过 / ${fail} 失败  (teardown 前 agents 序列化长度 ${before})`)
  await browser.close()
  process.exit(fail > 0 ? 1 : 0)
}
