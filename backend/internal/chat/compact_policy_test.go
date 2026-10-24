package chat

// REQ-396/M192：压缩策略解析单测——off=仅剪枝 / conn-id=指定连接 / 默认=agent 模型 / 不可解析回落。
import (
	"path/filepath"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/secrets"
	"github.com/xiaoyao/eino-multiagent-lab/backend/internal/store"
)

func TestResolveCompactAgent(t *testing.T) {
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	box, err := secrets.LoadKeyFile(filepath.Join(t.TempDir(), ".secret"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.CreateConnection(&store.ModelConnection{
		Name: "cheap", ConnType: "chat", Protocol: "openai_compat",
		BaseURL: "http://127.0.0.1:1", ModelName: "cheap", Enabled: true, IsDefault: true,
	}, nil); err != nil {
		t.Fatal(err)
	}
	assembler := &Assembler{Store: st, Box: box}
	s := &Service{Store: st, Assembler: assembler}

	// 默认（''）：返回原 agent，执行方 agent-model
	ag := &store.Agent{Name: "a"}
	eff, compactor := s.resolveCompactAgent(t.Context(), ag)
	if eff != ag || compactor != "agent-model" {
		t.Fatalf("默认应 agent-model: %+v %s", eff, compactor)
	}
	// off：nil + off
	ag2 := &store.Agent{Name: "b", CompactPolicy: "off"}
	eff2, compactor2 := s.resolveCompactAgent(t.Context(), ag2)
	if eff2 != nil || compactor2 != "off" {
		t.Fatalf("off 应 nil/off: %+v %s", eff2, compactor2)
	}
	// 指定连接：以该连接构造（可解析）→ low-cost:<id>
	ag3 := &store.Agent{Name: "c", CompactPolicy: "missing-conn"}
	eff3, compactor3 := s.resolveCompactAgent(t.Context(), ag3)
	if eff3 != nil || compactor3 != "off" {
		t.Fatalf("不可解析连接应诚实降级 off: %+v %s", eff3, compactor3)
	}
}
