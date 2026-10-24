# 真机凭证批机制（REQ-394/M190）——「待凭证」集中清账惯例

Q-21 工程化产品定位的可信度基建：散落在各档台账的「真机模型轮待凭证」条目（进化闭环/
A2A/委派/工作流/伴生/T1 基准等），在模型凭证/真机环境可用窗口**集中执行、结果批量回填**。

## 凭证要求

| 变量 | 用途 | 覆盖面 |
| --- | --- | --- |
| `DEEPSEEK_API_KEY` / 模型连接（设置页） | chat 模型轮 | 委派/工作流/进化诊断/伴生抽取 全链真机 |
| `DSH_S3_LIVE=1` | dsh SDK 真机回归门 | REQ-206 S3 TestSDKAdapterLiveRealDSH |
| `AGENTEVAL_AGENT_A/B` | agenteval A/B 评测 | REQ-223 评估 / REQ-386 T1 归因基准 |

## 执行序（run.sh 自动化）

1. `go test -tags eval ./...`（backend eval 组：伴生 gate/抽取）
2. `DSH_S3_LIVE=1 go test ./internal/inference/ -run Live`
3. agenteval 全量（BASE/AGENT_A/AGENT_B 注入）——REQ-223/REQ-386 T1
4. 真机冒烟链选跑（docs/20 对应条目：委派/工作流/进化/A2A）

## 结果回填纪律

执行后将各测试输出中的**实测数据**（时长/token/三率/事件数）回填至对应档迭代表与
docs/20 条目，把「真机待凭证」字样更新为实测值+日期；无法覆盖的条目如实维持待凭证。
