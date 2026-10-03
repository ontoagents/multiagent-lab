package ontobuild

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// REQ-267/M76：统一质量快评——构建平面 quality/check（inline spec+save=false）解析。
func TestQualitySnapshotParse(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/ontology/quality/check" {
			t.Fatalf("路径不符: %s", r.URL.Path)
		}
		var in map[string]any
		if err := json.NewDecoder(r.Body).Decode(&in); err != nil {
			t.Fatalf("请求体解析失败: %v", err)
		}
		if save, ok := in["save"].(bool); !ok || save {
			t.Fatalf("save 必须为 false（内存评分零副作用）: %v", in["save"])
		}
		if in["spec"] == nil {
			t.Fatalf("spec 内联必填")
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"report":{"error_count":1,"warning_count":2,"score":{"complete":80,"consistent":90,"maintainable":70,"overall":82.5}}}`))
	}))
	defer ts.Close()

	svc := NewService(nil, nil, nil)
	svc.BuildPlaneURL = ts.URL
	q := svc.QualitySnapshot(&buildSpec{Name: "t", Concepts: []buildConcept{{Name: "A"}}})
	if q.Degraded {
		t.Fatalf("构建平面可达不应降级: %+v", q)
	}
	if q.Overall != 82.5 || q.ErrorCount != 1 || q.WarningCount != 2 {
		t.Fatalf("快评解析不符: %+v", q)
	}
}

// 不可达降级：degraded=true 零值摘要，不 panic 不阻断（草稿生成本身不依赖构建平面存活）。
func TestQualitySnapshotDegraded(t *testing.T) {
	svc := NewService(nil, nil, nil)
	svc.BuildPlaneURL = "http://127.0.0.1:1" // 关闭端口
	q := svc.QualitySnapshot(&buildSpec{Name: "t", Concepts: []buildConcept{{Name: "A"}}})
	if !q.Degraded {
		t.Fatalf("不可达应降级: %+v", q)
	}
	if q.Overall != 0 || q.ErrorCount != 0 || q.WarningCount != 0 {
		t.Fatalf("降级摘要应为零值: %+v", q)
	}
}
