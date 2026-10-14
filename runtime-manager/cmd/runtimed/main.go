// runtimed 运行平面服务（方案 04 §4，:8090）：
// 运行方案编排 + Oxigraph 引擎适配 + 统一 MCP facade（/mcp，6 个 onto_* 工具：get_concept/get_instance/list_instances/neighbors/list_concepts/sparql_query——Q-14 契约 4→5→6）。
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"

	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine/fuseki"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/engine/oxigraph"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/facade"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/manager"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/rest"
	"github.com/xiaoyao/eino-multiagent-lab/runtime-manager/internal/store"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func main() {
	addr := env("ADDR", ":8090")
	dbPath := env("DB_PATH", "data/runtime.db")
	migrationsDir := env("MIGRATIONS_DIR", "migrations")
	buildURL := env("BUILD_SVC_URL", "http://127.0.0.1:8091")
	oxigraphBin := env("OXIGRAPH_BIN", "oxigraph_server")
	dataDir := env("ENGINE_DATA_DIR", "data/engines")
	logDir := env("ENGINE_LOG_DIR", "logs")

	// DB_PATH 父目录确保存在（filepath.Dir 兼容 / 与 \ 分隔符——Windows 桌面安装包 REQ-321 顺修，
	// 原手写 basename 仅识别 '/'，反斜杠路径会切出空串）
	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	st, err := store.Open(dbPath, migrationsDir)
	if err != nil {
		log.Fatalf("打开存储失败: %v", err)
	}
	eng := oxigraph.New(oxigraphBin, dataDir, logDir)
	mg := manager.New(st, buildURL, logDir, env("ENGINE_INSTALL_DIR", "data/bin"))
	mg.RegisterEngine("oxigraph", eng)
	// O6：Fuseki 引擎（FUSEKI_BIN 指向 fuseki-server 启动脚本；REQ-307 起常注册——未配置时经
	// 候选序动态解析（FUSEKI_BIN → PATH → data/bin），一键安装写入 data/bin 后即时命中无需重启）
	fusekiBin := env("FUSEKI_BIN", "")
	fusekiJava := env("FUSEKI_JAVA", "")
	if fusekiBin == "" {
		log.Printf("[runtimed] FUSEKI_BIN 未配置，fuseki 引擎以候选序动态解析（data/bin 一键安装落点）")
	} else if _, err := os.Stat(fusekiBin); err != nil {
		log.Printf("[runtimed] FUSEKI_BIN=%s 不存在，fuseki 引擎回退候选序动态解析", fusekiBin)
	}
	mg.RegisterEngine("fuseki", fuseki.New(fusekiBin, fusekiJava, dataDir, logDir))
	fc := facade.New(st, mg.ProcEndpoint)
	if env("TRACE_SPARQL", "on") == "off" { // 翻译透视开关（REQ-94，默认开）
		fc.TraceSparql = false
	}

	// REQ-236①/M63：启动对账——DB running 态逐一探测，存活引擎领养重建句柄、
	// 已死收敛 stopped（治重启后 procs 内存句柄丢失的僵尸 running/孤儿引擎双向漂移）。
	mg.Reconcile()

	mux := http.NewServeMux()
	mux.Handle("/mcp", fc.Mount())
	rest.New(st, mg).Mount(mux)

	log.Printf("[runtimed] 运行平面监听 %s (build=%s, oxigraph=%s)", addr, buildURL, oxigraphBin)
	log.Fatal(http.ListenAndServe(addr, mux))
}
