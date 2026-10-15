// launcher.json 可选配置（REQ-321 端口被占用问题的治本项）：
// 安装目录下放 launcher.json 可覆盖默认端口，用于与开发实例或其他占用者共存：
//   {"backend_port":18080, "ontology_port":18091, "runtime_port":18090}
// 三键均可省略（缺省 8080/8091/8090）。端口改动全链生效：服务 ADDR、backend 到本体两平面
// 的 BUILD_SVC_URL/RUNTIME_MGR_URL、ontologyd 回访 PLATFORM_URL、壳窗口加载地址。
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

type config struct {
	BackendPort  int `json:"backend_port"`
	OntologyPort int `json:"ontology_port"`
	RuntimePort  int `json:"runtime_port"`
}

// loadConfig 读安装目录 launcher.json（不存在/字段缺省/非法值 → 内建默认）。
func loadConfig(root string) config {
	cfg := config{BackendPort: 8080, OntologyPort: 8091, RuntimePort: 8090}
	b, err := os.ReadFile(filepath.Join(root, "launcher.json"))
	if err != nil {
		return cfg
	}
	var raw config
	if err := json.Unmarshal(b, &raw); err != nil {
		return cfg
	}
	if raw.BackendPort > 0 {
		cfg.BackendPort = raw.BackendPort
	}
	if raw.OntologyPort > 0 {
		cfg.OntologyPort = raw.OntologyPort
	}
	if raw.RuntimePort > 0 {
		cfg.RuntimePort = raw.RuntimePort
	}
	return cfg
}
