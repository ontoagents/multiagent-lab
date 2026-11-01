# 远程可访问部署（REQ-556/M363；107 号 §七）

面向「演示给外部人看」的远程部署参考档：Compose + Caddy（开源单二进制，D-O5）自动 HTTPS 反代，
外露面仅 80/443。**不改单机自部署产品边界**（Q-5 无账户体系；16 号 §11.4 不做清单维持）。

## 快速开始

```bash
# 1. 基础栈（三服务 + backend 127.0.0.1:8080 宿主本机入口）
docker compose -f docker-compose.yml up -d

# 2. 叠加远程档（追加 Caddy；域名经 DNS 指向本机后自动 ACME 出证书）
export DEMO_DOMAIN=demo.example.com
export PLATFORM_TOKEN=<强随机串>            # 外部访问凭证（全权）
export PLATFORM_TOKEN_RO=<另一强随机串>     # 可选：只读演示凭证（GET 全通/写 403/敏感导出 403）
docker compose -f docker-compose.yml -f remote/compose.remote.yml up -d

# 3. 验证外露面（仅 80/443 + 宿主本机 8080；卫星端口不发布）
docker compose ps
```

## 访问模型

| 入口 | 凭证 | 能力 |
| --- | --- | --- |
| 宿主本机 `127.0.0.1:8080` | 免（loopback 豁免，REQ-209） | 全权（讲解者/运维） |
| `https://$DEMO_DOMAIN` + 全权 token | `PLATFORM_TOKEN` | 全权 |
| `https://$DEMO_DOMAIN` + 只读 token | `PLATFORM_TOKEN_RO` | 仅 GET/HEAD/OPTIONS；写 403；敏感导出（backup/export）403；不补位 /mcp |
| 8090/8091/6333 | — | 不发布、不进反代（卫星零暴露，16 号 §11.3-4） |

只读动线（访客跟随）：本体浏览 → 资产 → 事件回放全 GET 链可走；现场交互演示（审批/插话）由
讲解者持全权 token 或本机操作。

## 注意

- **SSE**：Caddy 默认无缓冲直通（Caddyfile 已显式 `flush_interval -1`）。若换 nginx 反代，
  必须 `proxy_buffering off;`，否则事件流被缓冲、回放/审批卡延迟到达。
- **HTTPS 证书**：Caddy ACME 需要域名 DNS 指向本机且 80/443 可达；内网无公网形态可用
  Caddy `tls internal`（自签，访客浏览器需信任）或仅 HTTP 试验（改 Caddyfile 首行为 `:80`）。
- **seed**：演示资产灌装见 `../demo-seed.sh`（幂等）；重置 `../demo-reset.sh`。
- 修复注：compose/helm 已显式 `ADDR`（REQ-556——镜像默认回环经发布端口不可达的实测缺陷②③）；
  三镜像 Dockerfile 行内注续行断裂（实测缺陷①）已修。
