// ontologyd 构建平面服务（方案 04 §3，:8091）。
package main

import (
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/importer"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/repo"
	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/rest"
)

func env(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// resolveListenAddr REQ-383/M179：卫星服务默认回环监听——构建平面含未鉴权的治理/代理面
//（导入/LLM 生成/审计），默认仅 127.0.0.1 可达；局域网/容器部署显式 opt-in：
// LISTEN_ALL=1（绑全接口）或 ADDR 显式覆盖（显式值恒尊重，容器编排走此口）。
func resolveListenAddr(addrEnv, listenAll, port string) string {
	if addrEnv != "" {
		return addrEnv
	}
	if listenAll == "1" || listenAll == "true" {
		return ":" + port
	}
	return "127.0.0.1:" + port
}

func main() {
	addr := resolveListenAddr(os.Getenv("ADDR"), os.Getenv("LISTEN_ALL"), "8091")
	if !strings.HasPrefix(addr, "127.0.0.1") {
		log.Printf("[ontologyd] 监听 %s（非回环暴露——本服务管理面未鉴权，请确认网络隔离/反代；REQ-383）", addr)
	}
	dbPath := env("DB_PATH", "data/ontology.db")
	migrationsDir := env("MIGRATIONS_DIR", "migrations")
	platformURL := env("PLATFORM_URL", "http://127.0.0.1:8080")
	python := env("SIDECAR_PYTHON", "python3")
	script := env("SIDECAR_SCRIPT", "../tools/rdf-sidecar/sidecar.py")

	if err := os.MkdirAll(filepath.Dir(dbPath), 0o755); err != nil {
		log.Fatalf("创建数据目录失败: %v", err)
	}
	st, err := repo.Open(dbPath, migrationsDir)
	if err != nil {
		log.Fatalf("打开仓库失败: %v", err)
	}
	// REQ-381②/M177：启动对账——悬挂 async_job running 标 error（进程重启 goroutine 消失，诚实报错重新发起）
	if n, err := st.MarkOrphanAsyncJobs(); err == nil && n > 0 {
		log.Printf("[ontologyd] 启动对账：%d 条悬挂异步任务标 error（重启中断，可重新发起）", n)
	}
	sc := &importer.Sidecar{Python: python, Script: script}
	llm := llmcreate.New(platformURL)

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	rest.New(st, sc, llm).Mount(mux)

	log.Printf("[ontologyd] 构建平面监听 %s (db=%s, platform=%s, sidecar=%s %s)", addr, dbPath, platformURL, python, script)
	log.Fatal(http.ListenAndServe(addr, mux))
}
