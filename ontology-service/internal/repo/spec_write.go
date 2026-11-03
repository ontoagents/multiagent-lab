package repo

import (
	"database/sql"
	"errors"
	"fmt"
)

// SpecWrite is one version-conditioned change to the head and its edit history.
type SpecWrite struct {
	ExpectedVersion          int
	SpecJSON                 string
	OriginalFormat           string
	OriginalContent          string
	Overlays                 []*ObjectOverlay
	RemoveOverlayIDs         []string
	ClearOverlays            bool
	RefreshOverlayBase       map[string]string
	ExpectedOverlayRevisions map[string]int
}

type SpecSnapshot struct {
	Version     int
	SpecJSON    string
	Name        string
	Description string
}

// ReadSpecSnapshot reads metadata and content in the same SQLite statement.
// An ontology without a normalized artifact has an empty SpecJSON.
func (s *Store) ReadSpecSnapshot(id string) (*SpecSnapshot, error) {
	o := &SpecSnapshot{}
	err := s.db.QueryRow(`SELECT o.version,COALESCE(a.content,''),o.name,o.description
		FROM ontology o LEFT JOIN ontology_artifact a ON a.ontology_id=o.id AND a.format='spec_json'
		WHERE o.id=?`, id).Scan(&o.Version, &o.SpecJSON, &o.Name, &o.Description)
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return o, err
}

func checkHeadUpdate(tx *sql.Tx, id string, res sql.Result) error {
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n != 0 {
		return nil
	}
	var found int
	if err := tx.QueryRow(`SELECT 1 FROM ontology WHERE id=?`, id).Scan(&found); errors.Is(err, sql.ErrNoRows) {
		return ErrNotFound
	} else if err != nil {
		return err
	}
	return fmt.Errorf("%w: 本体已变更，请刷新后重试", ErrConflict)
}

func (s *Store) SaveSpecSnapshot(id string, w SpecWrite) (int, error) {
	if w.ExpectedVersion <= 0 {
		return 0, fmt.Errorf("%w: expected version must be positive", ErrConflict)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	// The first statement takes the writer lock; no deferred read→write upgrade.
	res, err := tx.Exec(`UPDATE ontology SET version=version+1,status='draft',version_name='',updated_at=CURRENT_TIMESTAMP
		WHERE id=? AND version=?`, id, w.ExpectedVersion)
	if err != nil {
		return 0, err
	}
	if err := checkHeadUpdate(tx, id, res); err != nil {
		return 0, err
	}
	v := w.ExpectedVersion + 1
	if _, err := tx.Exec(`INSERT INTO ontology_artifact(ontology_id,format,content,is_normalized,imported_at)
		VALUES(?,'spec_json',?,1,CURRENT_TIMESTAMP) ON CONFLICT(ontology_id,format)
		DO UPDATE SET content=excluded.content,is_normalized=1,imported_at=CURRENT_TIMESTAMP`, id, w.SpecJSON); err != nil {
		return 0, err
	}
	// Never overwrite a frozen historical version.
	if _, err := tx.Exec(`INSERT INTO ontology_version(ontology_id,version,spec_json,original_format,original_content,created_at)
		VALUES(?,?,?,?,?,CURRENT_TIMESTAMP)`, id, v, w.SpecJSON, w.OriginalFormat, w.OriginalContent); err != nil {
		return 0, err
	}
	if w.ClearOverlays {
		if _, err := tx.Exec(`DELETE FROM ontology_object_overlay WHERE ontology_id=?`, id); err != nil {
			return 0, err
		}
	}
	for key, base := range w.RefreshOverlayBase {
		if base == "" {
			continue
		}
		if _, err := tx.Exec(`UPDATE ontology_object_overlay SET base_json=? WHERE ontology_id=? AND kind||':'||name=?`, base, id, key); err != nil {
			return 0, err
		}
	}
	for _, lid := range w.RemoveOverlayIDs {
		if _, err := tx.Exec(`DELETE FROM ontology_object_overlay WHERE ontology_id=? AND logical_id=?`, id, lid); err != nil {
			return 0, err
		}
	}
	saved := make([]*ObjectOverlay, 0, len(w.Overlays))
	for _, o := range w.Overlays {
		if o.OntologyID != id {
			return 0, fmt.Errorf("overlay ontology mismatch")
		}
		row, err := upsertOverlayTx(tx, o, w.ExpectedOverlayRevisions[o.LogicalID])
		if err != nil {
			return 0, err
		}
		saved = append(saved, row)
	}
	if err := tx.Commit(); err != nil {
		return 0, err
	}
	for i, row := range saved {
		*w.Overlays[i] = *row
	}
	return v, nil
}
