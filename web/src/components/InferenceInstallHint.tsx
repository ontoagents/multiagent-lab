import { useMemo, useState } from 'react'
import { Button, Popover, Space, Typography } from 'antd'
import { api } from '../api/client'
import type { InferenceBackendStatus } from '../api/types'

/**
 * REQ-450：未安装推理后端安装引导（引导安装非代为安装——平台不下载不托管）。
 * 未发现（PATH 探测失败）后端 ≥1 时在推理后端字段下方出「安装指引」入口：
 * Popover 逐后端列安装命令（可复制）/官方文档/说明，装好后「重新探测」即点即刷
 * （POST /api/inference-backends/reprobe，沿设置页「推理后端」面板同款）。
 */
export default function InferenceInstallHint({
  backends,
  onReprobed,
}: {
  backends?: InferenceBackendStatus[]
  onReprobed?: (backends: InferenceBackendStatus[]) => void
}) {
  const [open, setOpen] = useState(false)
  const [probing, setProbing] = useState(false)
  const missing = useMemo(
    () => (backends ?? []).filter((b) => !b.default && !b.available),
    [backends],
  )
  if (missing.length === 0) return null

  const reprobe = async () => {
    setProbing(true)
    try {
      const r = await api.reprobeInferenceBackends()
      onReprobed?.(r.backends ?? [])
    } catch {
      // 探测失败保持现清单（保底语义，不弹错打断配置动线）
    } finally {
      setProbing(false)
    }
  }

  return (
    <Popover
      open={open}
      onOpenChange={setOpen}
      trigger="click"
      placement="bottom"
      overlayStyle={{ maxWidth: 420 }}
      content={
        <div style={{ maxWidth: 380 }}>
          {missing.map((b) => (
            <div key={b.name} style={{ marginBottom: 10 }}>
              <Typography.Text strong style={{ fontSize: 13 }}>{b.name}</Typography.Text>
              {b.install_cmd ? (
                <div style={{ marginTop: 2 }}>
                  <Typography.Text code copyable={{ text: b.install_cmd }} style={{ fontSize: 12 }}>
                    {b.install_cmd}
                  </Typography.Text>
                </div>
              ) : null}
              {b.install_docs ? (
                <div>
                  <a href={b.install_docs} target="_blank" rel="noreferrer" style={{ fontSize: 12 }}>
                    官方安装文档 ↗
                  </a>
                </div>
              ) : null}
              {b.install_note ? (
                <div>
                  <Typography.Text type="secondary" style={{ fontSize: 12 }}>{b.install_note}</Typography.Text>
                </div>
              ) : null}
            </div>
          ))}
          <Space size={8} style={{ marginTop: 2 }}>
            <Button size="small" loading={probing} onClick={reprobe}>重新探测</Button>
            <Typography.Text type="secondary" style={{ fontSize: 12 }}>
              装好后点此刷新（也可到「设置 → 推理后端」）
            </Typography.Text>
          </Space>
        </div>
      }
    >
      <Button
        type="link"
        size="small"
        style={{ padding: 0, height: 'auto', fontSize: 12 }}
        onClick={(e) => e.stopPropagation()}
      >
        {missing.length} 个推理后端未发现（未安装或未加入 PATH）· 安装指引
      </Button>
    </Popover>
  )
}
