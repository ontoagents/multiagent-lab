package repo

import (
	"errors"
	"sync"
	"testing"
)

func TestAtomicSpecWriteConcurrentCAS(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "atomic", `{"name":"before"}`)
	start := make(chan struct{})
	results := make(chan error, 2)
	var wg sync.WaitGroup
	for _, body := range []string{`{"name":"one"}`, `{"name":"two"}`} {
		wg.Add(1)
		go func(body string) {
			defer wg.Done()
			<-start
			_, err := st.SaveSpecSnapshot("atomic", SpecWrite{ExpectedVersion: 1, SpecJSON: body})
			results <- err
		}(body)
	}
	close(start)
	wg.Wait()
	close(results)
	success, conflict := 0, 0
	for err := range results {
		if err == nil {
			success++
		} else if errors.Is(err, ErrConflict) {
			conflict++
		} else {
			t.Fatalf("unexpected result: %v", err)
		}
	}
	if success != 1 || conflict != 1 {
		t.Fatalf("success=%d conflict=%d", success, conflict)
	}
	snap, err := st.ReadSpecSnapshot("atomic")
	if err != nil {
		t.Fatal(err)
	}
	history, err := st.GetVersionSpec("atomic", 2)
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != 2 || history != snap.SpecJSON {
		t.Fatalf("head and version snapshot disagree: %+v / %s", snap, history)
	}
}

func TestAtomicSpecWriteRollsBackFault(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "atomic", `{"name":"before"}`)
	if _, err := st.Publish("atomic", "v1"); err != nil {
		t.Fatal(err)
	}
	row := &ObjectOverlay{OntologyID: "atomic", LogicalID: "old", Kind: "concept", Name: "Device", Action: "edit", OverlayJSON: "{}"}
	if _, err := st.UpsertOverlay(row, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := st.db.Exec(`CREATE TRIGGER fail_snapshot BEFORE INSERT ON ontology_version WHEN NEW.version=2 BEGIN SELECT RAISE(ABORT,'injected snapshot failure'); END`); err != nil {
		t.Fatal(err)
	}
	_, err := st.SaveSpecSnapshot("atomic", SpecWrite{ExpectedVersion: 1, SpecJSON: `{"name":"after"}`, ClearOverlays: true})
	if err == nil {
		t.Fatal("fault must fail the save")
	}
	snap, err := st.ReadSpecSnapshot("atomic")
	if err != nil {
		t.Fatal(err)
	}
	o, err := st.GetOntology("atomic")
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != 1 || snap.SpecJSON != `{"name":"before"}` || o.Status != "published" || o.VersionName != "v1" {
		t.Fatalf("partial save: snapshot=%+v ontology=%+v", snap, o)
	}
	if _, err := st.GetOverlay("atomic", "old"); err != nil {
		t.Fatalf("failed save cleared overlay: %v", err)
	}
}

func TestAtomicSpecWriteDoesNotOverwriteSnapshot(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "atomic", `{"name":"before"}`)
	if err := st.SaveVersion("atomic", 2, `{"name":"frozen"}`, "", ""); err != nil {
		t.Fatal(err)
	}
	if _, err := st.SaveSpecSnapshot("atomic", SpecWrite{ExpectedVersion: 1, SpecJSON: `{"name":"after"}`}); err == nil {
		t.Fatal("existing snapshot must not be overwritten")
	}
	got, _ := st.GetVersionSpec("atomic", 2)
	snap, _ := st.ReadSpecSnapshot("atomic")
	if got != `{"name":"frozen"}` || snap.Version != 1 || snap.SpecJSON != `{"name":"before"}` {
		t.Fatalf("snapshot drift: %s %+v", got, snap)
	}
}

func TestAtomicPublishRejectsStaleContent(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "atomic", `{"name":"before"}`)
	if _, err := st.SaveSpecSnapshot("atomic", SpecWrite{ExpectedVersion: 1, SpecJSON: `{"name":"after"}`}); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishReleaseAt("atomic", "release", 1, `{"name":"before"}`, "old ttl", 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale publish err=%v", err)
	}
	if _, err := st.PublishReleaseAt("atomic", "release", 2, `{"name":"before"}`, "old ttl", 1); !errors.Is(err, ErrConflict) {
		t.Fatalf("wrong content publish err=%v", err)
	}
	if rels, _ := st.ListReleases("atomic"); len(rels) != 0 {
		t.Fatalf("stale publish left release: %+v", rels)
	}
}

func TestAtomicPublishRollsBackStatusFault(t *testing.T) {
	st := newReleaseStore(t)
	seedOntology(t, st, "atomic", `{"name":"before"}`)
	if _, err := st.db.Exec(`CREATE TRIGGER fail_publish BEFORE UPDATE OF status ON ontology WHEN NEW.status='published' BEGIN SELECT RAISE(ABORT,'injected publish failure'); END`); err != nil {
		t.Fatal(err)
	}
	if _, err := st.PublishReleaseAt("atomic", "release", 1, `{"name":"before"}`, "ttl", 1); err == nil {
		t.Fatal("fault must fail publishing")
	}
	if rels, _ := st.ListReleases("atomic"); len(rels) != 0 {
		t.Fatalf("partial publication: %+v", rels)
	}
	o, _ := st.GetOntology("atomic")
	if o.Status != "draft" {
		t.Fatalf("unexpected status %q", o.Status)
	}
}
