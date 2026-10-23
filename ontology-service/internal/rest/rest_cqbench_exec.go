package rest

// REQ-382/M178：cqbench 执行级对齐通道——RUNTIME_MGR_URL 指向运行平面
// POST /api/ontology-sparql（按本体路由执行只读 SPARQL）。不可达时返回错误，
// cqbench.Run 回落文本子串口径并如实标注 gold_align_mode。

import (
	"os"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/cqbench"
)

// runtimeMgrURL 运行平面基址：Server.RuntimeURL（构造注入）优先，env 兜底，默认本机 :8090。
func (s *Server) runtimeMgrURL() string {
	if s != nil && s.RuntimeURL != "" {
		return s.RuntimeURL
	}
	if v := os.Getenv("RUNTIME_MGR_URL"); v != "" {
		return v
	}
	return "http://127.0.0.1:8090"
}

// benchExecutor 构造执行通道（每次调用新建 client——job 后台 goroutine 内使用）。
func (s *Server) benchExecutor() cqbench.Executor {
	return func(ctx context.Context, ontologyID, query string) ([]map[string]any, error) {
		body, _ := json.Marshal(map[string]string{"ontology_id": ontologyID, "query": query})
		req, err := http.NewRequestWithContext(ctx, http.MethodPost, strings.TrimRight(s.runtimeMgrURL(), "/")+"/api/ontology-sparql", bytes.NewReader(body))
		if err != nil {
			return nil, err
		}
		req.Header.Set("Content-Type", "application/json")
		client := &http.Client{Timeout: 30 * time.Second}
		resp, err := client.Do(req)
		if err != nil {
			return nil, fmt.Errorf("运行平面不可达: %w", err)
		}
		defer resp.Body.Close()
		var res struct {
			Bindings []map[string]any `json:"bindings"`
			Error    string           `json:"error"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
			return nil, fmt.Errorf("运行平面响应解析失败: %w", err)
		}
		if res.Error != "" {
			return nil, fmt.Errorf("执行失败: %s", res.Error)
		}
		return res.Bindings, nil
	}
}
