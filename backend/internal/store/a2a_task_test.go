// REQ-418/M214：ListA2ATasks 列表（倒序/limit/集合在册）。
package store

import (
	"fmt"
	"path/filepath"
	"testing"
)

func newA2ATaskTestStore(t *testing.T) *Store {
	t.Helper()
	st, err := Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestListA2ATasks(t *testing.T) {
	st := newA2ATaskTestStore(t)
	for i := 0; i < 3; i++ {
		if err := st.UpsertA2ATask(&A2ATaskRow{ID: fmt.Sprintf("t%d", i), ContextID: "c", AgentID: "a", State: "completed", Payload: fmt.Sprintf(`{"artifact_text":"产物%d"}`, i)}); err != nil {
			t.Fatal(err)
		}
	}
	rows, err := st.ListA2ATasks(0)
	if err != nil {
		t.Fatal(err)
	}
	if len(rows) != 3 {
		t.Fatalf("want 3 rows, got %d", len(rows))
	}
	ids := map[string]bool{}
	for _, r := range rows {
		ids[r.ID] = true
	}
	for _, want := range []string{"t0", "t1", "t2"} {
		if !ids[want] {
			t.Fatalf("缺 %s: %v", want, ids)
		}
	}
	if rows2, _ := st.ListA2ATasks(2); len(rows2) != 2 {
		t.Fatalf("limit=2 want 2, got %d", len(rows2))
	}
}
