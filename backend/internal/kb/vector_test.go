// REQ-593 向量后端启动探测自动回退单测：KB-O1「QDRANT_URL 缺失/不可达可退 sqlite」实现补齐——
// 不可达回退 / URL 缺失回退 / 显式 sqlite 直取 / 未知值报错 / httptest 可达返回 QdrantStore /
// Service 全链切换（backendName→StoreBackend 写入口径、Healthz→sqlite-ok）。
package kb

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ontoagents/ontoagents/backend/internal/store"
)

func newVectorTestStore(t *testing.T) *store.Store {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "vector-test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

// TestNewVectorStoreFallbackUnreachable 不可达（connection refused）→ SQLite fallback（缺陷主场景：
// 本机无 Qdrant，原实现返回 QdrantStore，索引/删除/检索全链 dial 硬错）。
func TestNewVectorStoreFallbackUnreachable(t *testing.T) {
	vs, err := NewVectorStore("qdrant", "http://127.0.0.1:1", newVectorTestStore(t))
	if err != nil {
		t.Fatalf("fallback expected, got error: %v", err)
	}
	if _, ok := vs.(SQLiteStore); !ok {
		t.Fatalf("expected SQLiteStore fallback, got %T", vs)
	}
}

// TestNewVectorStoreFallbackMissingURL URL 缺失 → 回退（KB-O1 口径「缺失可退」；原实现硬错误）。
func TestNewVectorStoreFallbackMissingURL(t *testing.T) {
	vs, err := NewVectorStore("qdrant", "", newVectorTestStore(t))
	if err != nil {
		t.Fatalf("fallback expected, got error: %v", err)
	}
	if _, ok := vs.(SQLiteStore); !ok {
		t.Fatalf("expected SQLiteStore fallback, got %T", vs)
	}
	// 空串=默认值路径（getenv 缺省 qdrant+QDRANT_URL 可能未配）同语义
	vs2, err := NewVectorStore("", "", newVectorTestStore(t))
	if err != nil {
		t.Fatalf("default path fallback expected, got error: %v", err)
	}
	if _, ok := vs2.(SQLiteStore); !ok {
		t.Fatalf("expected SQLiteStore fallback, got %T", vs2)
	}
}

// TestNewVectorStoreExplicitSQLite 显式 sqlite 零探测直取。
func TestNewVectorStoreExplicitSQLite(t *testing.T) {
	vs, err := NewVectorStore("sqlite", "", newVectorTestStore(t))
	if err != nil {
		t.Fatalf("explicit sqlite: %v", err)
	}
	if _, ok := vs.(SQLiteStore); !ok {
		t.Fatalf("expected SQLiteStore, got %T", vs)
	}
}

// TestNewVectorStoreUnknownBackend 未知后端值仍报错（配置错误不静默）。
func TestNewVectorStoreUnknownBackend(t *testing.T) {
	if _, err := NewVectorStore("memory", "", newVectorTestStore(t)); err == nil {
		t.Fatalf("unknown backend should error")
	}
}

// TestNewVectorStoreQdrantReachable 可达（httptest /collections 200）→ 正常返回 QdrantStore 不回退。
func TestNewVectorStoreQdrantReachable(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"result":{"collections":[]}}`))
	}))
	defer srv.Close()
	vs, err := NewVectorStore("qdrant", srv.URL, newVectorTestStore(t))
	if err != nil {
		t.Fatalf("reachable qdrant: %v", err)
	}
	if _, ok := vs.(*QdrantStore); !ok {
		t.Fatalf("expected *QdrantStore, got %T", vs)
	}
}

// TestServiceFallbackWholeChain Service 级全链切换：qdrant 不可达装配 → backendName=sqlite
//（索引 StoreBackend 列/BLOB 写入口径随类型开关）+ Healthz=sqlite-ok（原缺陷场景下为 qdrant-unreachable）。
func TestServiceFallbackWholeChain(t *testing.T) {
	svc, err := NewService(newVectorTestStore(t), nil, "qdrant", "http://127.0.0.1:1")
	if err != nil {
		t.Fatalf("service with fallback: %v", err)
	}
	if got := svc.backendName(); got != "sqlite" {
		t.Fatalf("backendName = %q, want sqlite", got)
	}
	if got := svc.Healthz(context.Background()); got != "sqlite-ok" {
		t.Fatalf("Healthz = %q, want sqlite-ok", got)
	}
	// fallback 后删除链为 no-op（原缺陷场景 DeleteDoc 会打 points/delete 连接拒错误日志）
	if err := svc.Vector.DeleteByDoc(context.Background(), "kb_x", "doc_x"); err != nil {
		t.Fatalf("fallback DeleteByDoc should be no-op, got %v", err)
	}
}
