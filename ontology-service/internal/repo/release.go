package repo

// REQ-541/M348：Publish 不可变发布快照（103 号《本体依赖管理与仓库服务调研》§6.4 发布件/
// §6.5 步骤一，修痛点 P1 载体面）——OBO FP-004「发布文件内容 MUST NOT 修改」+ OCI digest
// 内容寻址思想：版本表即 tag（(ontology_id, version) 唯一定位一份快照），内容列一经写入
// 不可变（代码层无其 UPDATE 路径），同版本重发布同内容幂等复用（同 digest），内容漂移
// ErrConflict 拒绝；快照不做预物化产物（owlrl N²~N³ 膨胀在案观察维持，103 号 R3）。
// 快照自带依赖账本 manifest（pin 解析后依赖闭包清单，BFS 语义与运行面 expandClosure 同构）。

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
)

// ReleaseInfo 发布快照元信息（不含内容 blob——发布响应/版本列表/清单透出）。
type ReleaseInfo struct {
	Version     int    `json:"version"`
	VersionName string `json:"version_name,omitempty"`
	SpecSHA256  string `json:"spec_sha256"`
	TTLSHA256   string `json:"ttl_sha256"`
	Triples     int    `json:"triples"` // 发布时 TTL 精确三元组数；-1=未知（axioms-raw 余量拼接）
	PublishedAt string `json:"published_at"`
}

// ReleaseSummary 发布快照清单项（元信息+依赖账本；内容 blob 走 artifact 端点按需取）。
type ReleaseSummary struct {
	ReleaseInfo
	OntologyID   string `json:"ontology_id,omitempty"`
	ManifestJSON string `json:"manifest_json,omitempty"`
}

// ReleaseRecord 完整发布快照（含冻结内容——不可变工件本体）。
type ReleaseRecord struct {
	ReleaseSummary
	OntologyID string `json:"ontology_id"`
	SpecJSON   string `json:"spec_json"`
	TTL        string `json:"ttl"`
}

// ManifestDependency 依赖账本一条（发布时点解析结果）。
type ManifestDependency struct {
	OntoID     string `json:"onto_id"`
	Version    int    `json:"version"`               // 0=发布时点目标本体已删（missing）
	Resolution string `json:"resolution"`            // pin | head | missing
	SpecSHA256 string `json:"spec_sha256,omitempty"` // 该版本内容指纹（快照/工件缺失为空）
}

// ReleaseManifest 发布快照依赖账本（pin 解析后依赖闭包清单，103 号 §6.4「依赖账本件」）。
// 诚实边界：external_refs 登记本身无版本史（当前态旁挂表），闭包遍历用发布时点登记；
// 仅记账不阻断——悬空目标以 resolution=missing 如实入账（装载阻断仍是运行面门禁职责）。
type ReleaseManifest struct {
	OntoID        string               `json:"ontology_id"`
	Version       int                  `json:"version"`
	Dependencies  []ManifestDependency `json:"dependencies"`
	RemoteSkipped []string             `json:"remote_skipped,omitempty"` // 外部 IRI 登记态（不拉取，D-O27）
}

// sha256Hex 内容寻址 digest（OCI 思想：内容 bytes → sha256 hex；字段名即算法名）。
func sha256Hex(content string) string {
	sum := sha256.Sum256([]byte(content))
	return hex.EncodeToString(sum[:])
}

// shortSum 错误消息用 digest 缩略（前 12 位足够定位，完整值以快照行与响应为准）。
func shortSum(sum string) string {
	if len(sum) > 12 {
		return sum[:12] + "…"
	}
	return sum
}

// urnOntoIDOf owl:imports 哨兵登记的 urn:o:{id}: 形态 IRI → 仓内本体 ID；非该形态=远端
// （空串，记入 remote_skipped 不拉取，D-O27）。与运行面 manager.urnOntoOf 同构（ID 含
// sanitize 变形字符时解析失配按远端跳过——同款诚实边界，构造侧见 importer.basePrefixOf）。
func urnOntoIDOf(iri string) string {
	iri = strings.TrimSpace(iri)
	if !strings.HasPrefix(iri, "urn:o:") || !strings.HasSuffix(iri, ":") {
		return ""
	}
	id := strings.TrimSuffix(strings.TrimPrefix(iri, "urn:o:"), ":")
	if id == "" || strings.ContainsAny(id, " <>\"{}|\\^`") {
		return ""
	}
	return id
}

// PublishRelease 发布当前版本并冻结不可变发布快照（spec_json+TTL+sha256 digest+依赖账本
// manifest 落库，版本表即 tag），随后沿 REQ-239 既有语义置 published+命名。
// 不可变语义（OBO FP-004）：同版本已有快照时——内容 digest 一致=幂等复用既有行（同 digest、
// published_at 不变，仅 version_name tag 标签可随重发布刷新，OCI tag 可变名语义）；
// digest 不一致=内容漂移，拒绝（ErrConflict）且不改任何状态——版本内容须演进（保存触发
// bump）后发布新版本，或还原漂移变更。
func (s *Store) PublishRelease(id, versionName, specJSON, ttl string, triples int) (*Ontology, error) {
	head, err := s.ReadSpecSnapshot(id)
	if err != nil {
		return nil, err
	}
	return s.PublishReleaseAt(id, versionName, head.Version, specJSON, ttl, triples)
}

// PublishReleaseAt freezes only the exact head used to produce the exported TTL.
func (s *Store) PublishReleaseAt(id, versionName string, expectedVersion int, specJSON, ttl string, triples int) (*Ontology, error) {
	name := strings.TrimSpace(versionName)
	if name == "" {
		name = fmt.Sprintf("v%d", expectedVersion)
	}
	manifest, err := s.buildReleaseManifest(id, expectedVersion)
	if err != nil {
		return nil, err
	}
	mj, err := json.Marshal(manifest)
	if err != nil {
		return nil, err
	}
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()
	// Lock before reading; both version and exported input must still match.
	res, err := tx.Exec(`UPDATE ontology SET version=version WHERE id=? AND version=? AND
		EXISTS(SELECT 1 FROM ontology_artifact WHERE ontology_id=? AND format='spec_json' AND content=?)`,
		id, expectedVersion, id, specJSON)
	if err != nil {
		return nil, err
	}
	if err := checkHeadUpdate(tx, id, res); err != nil {
		return nil, err
	}
	specSum, ttlSum := sha256Hex(specJSON), sha256Hex(ttl)
	prev, err := scanReleaseRow(tx.QueryRow(`SELECT `+releaseFullCols+` FROM ontology_release WHERE ontology_id=? AND version=?`, id, expectedVersion))
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return nil, err
	}
	if prev != nil {
		if prev.SpecSHA256 != specSum || prev.TTLSHA256 != ttlSum {
			return nil, fmt.Errorf("%w: 本体 %s v%d 已有不可变发布快照且 digest 不一致（OBO FP-004：发布内容 MUST NOT 修改），请保存为新版本后发布", ErrConflict, id, expectedVersion)
		}
	} else {
		if _, err := tx.Exec(`INSERT INTO ontology_release
			(ontology_id,version,version_name,spec_json,ttl,manifest_json,spec_sha256,ttl_sha256,triples,published_at)
			VALUES(?,?,?,?,?,?,?,?,?,CURRENT_TIMESTAMP)`, id, expectedVersion, name, specJSON, ttl, string(mj), specSum, ttlSum, triples); err != nil {
			return nil, err
		}
	}
	if _, err := tx.Exec(`UPDATE ontology_release SET version_name=? WHERE ontology_id=? AND version=?`, name, id, expectedVersion); err != nil {
		return nil, err
	}
	if _, err := tx.Exec(`UPDATE ontology SET status='published',version_name=?,updated_at=CURRENT_TIMESTAMP WHERE id=?`, name, id); err != nil {
		return nil, err
	}
	o := &Ontology{}
	if err := tx.QueryRow(`SELECT id,name,description,version,IFNULL(forked_from,''),created_at,updated_at,quality_strict,IFNULL(status,'draft'),IFNULL(version_name,''),IFNULL(reasoning_check,0) FROM ontology WHERE id=?`, id).
		Scan(&o.ID, &o.Name, &o.Description, &o.Version, &o.ForkedFrom, &o.CreatedAt, &o.UpdatedAt, &o.QualityStrict, &o.Status, &o.VersionName, &o.ReasoningCheck); err != nil {
		return nil, err
	}
	rel, err := scanReleaseRow(tx.QueryRow(`SELECT `+releaseFullCols+` FROM ontology_release WHERE ontology_id=? AND version=?`, id, expectedVersion))
	if err != nil {
		return nil, err
	}
	o.Release = &rel.ReleaseInfo
	if err := tx.Commit(); err != nil {
		return nil, err
	}
	return o, nil
}

// buildReleaseManifest 发布时点依赖闭包账本：BFS 沿 import/reuse 仓内引用 + owl:imports 哨兵
// 登记（FromConcept="(ontology)"，to_concept=urn:o:{id}:）递归补全——环 visited 收敛=并集
// 装载语义（与运行面 expandClosure 同构）。版本解析按 REQ-540 pin 语义：同目标多登记 pin
// 取最低（最保守解析，不限 ref_kind——登记即 pin 载体），无 pin 记发布时点头版本；
// 悬空目标（本体已删）resolution=missing 如实入账不阻断发布。依赖序=BFS 发现序（引用清单
// 按创建稳定序），仅装载边（import/reuse）入闭包，alignment 纯登记不产内容不入闭包。
func (s *Store) buildReleaseManifest(ontoID string, version int) (*ReleaseManifest, error) {
	m := &ReleaseManifest{OntoID: ontoID, Version: version, Dependencies: []ManifestDependency{}}
	visited := map[string]bool{ontoID: true}
	queue := []string{ontoID}
	pins := map[string]int{} // 目标 → 最低 pin（同目标多登记取最低，与运行面收敛口径一致）
	remoteSeen := map[string]bool{}
	missing := map[string]bool{} // 发布时点已删目标（不沿其孤儿引用外扩——运行面悬空处即阻断，账本同构）
	var deps []string            // BFS 发现序（不含根本体）
	for len(queue) > 0 {
		oid := queue[0]
		queue = queue[1:]
		if _, err := s.GetOntology(oid); err != nil {
			if errors.Is(err, ErrNotFound) {
				missing[oid] = true
				continue
			}
			return nil, err
		}
		refs, err := s.ListExternalRefs(oid)
		if err != nil {
			return nil, fmt.Errorf("依赖账本构建失败（本体 %s 引用清单读取）: %w", oid, err)
		}
		for _, ref := range refs {
			if ref.ToOntoID != "" && ref.PinVersion > 0 {
				if prev, ok := pins[ref.ToOntoID]; !ok || ref.PinVersion < prev {
					pins[ref.ToOntoID] = ref.PinVersion
				}
			}
			switch {
			case ref.ToOntoID != "" && (ref.RefKind == "import" || ref.RefKind == "reuse"):
				if !visited[ref.ToOntoID] {
					visited[ref.ToOntoID] = true
					deps = append(deps, ref.ToOntoID)
					queue = append(queue, ref.ToOntoID)
				}
			case ref.ToOntoID == "" && ref.RefKind == "import" && ref.FromConcept == OntologyLevelRef:
				if tgt := urnOntoIDOf(ref.ToConcept); tgt != "" {
					if !visited[tgt] {
						visited[tgt] = true
						deps = append(deps, tgt)
						queue = append(queue, tgt)
					}
				} else if !remoteSeen[ref.ToConcept] {
					remoteSeen[ref.ToConcept] = true
					m.RemoteSkipped = append(m.RemoteSkipped, ref.ToConcept)
				}
			}
		}
	}
	for _, dep := range deps {
		d := ManifestDependency{OntoID: dep}
		switch {
		case missing[dep]:
			d.Resolution = "missing" // 发布时点目标已删（悬空如实记账，装载阻断在运行面门禁）
		case pins[dep] > 0:
			d.Version, d.Resolution = pins[dep], "pin"
			// pin 快照指纹（登记时校验过存在；缺失为空不虚构）
			if spec, err := s.GetVersionSpec(dep, d.Version); err == nil {
				d.SpecSHA256 = sha256Hex(spec)
			} else if !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		default:
			o, err := s.GetOntology(dep)
			if err != nil {
				if errors.Is(err, ErrNotFound) {
					d.Resolution = "missing"
					break
				}
				return nil, err
			}
			d.Version, d.Resolution = o.Version, "head"
			// head 指纹=发布时点该本体头版本 artifact 内容（artifact 恒头版本内容，与
			// ontology.version 同源；工件缺失为空——仅有 original 未归一化的外部本体态）
			if spec, _, err := s.GetArtifact(dep, "spec_json"); err == nil {
				d.SpecSHA256 = sha256Hex(spec)
			} else if !errors.Is(err, ErrNotFound) {
				return nil, err
			}
		}
		m.Dependencies = append(m.Dependencies, d)
	}
	return m, nil
}

const releaseFullCols = `ontology_id,version,version_name,spec_json,ttl,manifest_json,spec_sha256,ttl_sha256,triples,published_at`

// scanReleaseRow 完整行扫描（GetRelease 用）。
func scanReleaseRow(row interface{ Scan(...any) error }) (*ReleaseRecord, error) {
	r := &ReleaseRecord{}
	if err := row.Scan(&r.OntologyID, &r.Version, &r.VersionName, &r.SpecJSON, &r.TTL, &r.ManifestJSON,
		&r.SpecSHA256, &r.TTLSHA256, &r.Triples, &r.PublishedAt); err != nil {
		return nil, err
	}
	return r, nil
}

// scanReleaseMeta 元信息行扫描（清单/单查用，不含内容 blob）。
func scanReleaseMeta(row interface{ Scan(...any) error }) (*ReleaseSummary, error) {
	r := &ReleaseSummary{}
	if err := row.Scan(&r.Version, &r.VersionName, &r.SpecSHA256, &r.TTLSHA256, &r.Triples, &r.PublishedAt, &r.ManifestJSON); err != nil {
		return nil, err
	}
	return r, nil
}

// GetRelease 完整发布快照（含冻结内容 blob——digest 校验/工件直出用）。
func (s *Store) GetRelease(ontologyID string, version int) (*ReleaseRecord, error) {
	r, err := scanReleaseRow(s.db.QueryRow(`SELECT `+releaseFullCols+` FROM ontology_release WHERE ontology_id=? AND version=?`, ontologyID, version))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	return r, err
}

// GetReleaseMeta 单份发布快照元信息+依赖账本（不含内容 blob）。
func (s *Store) GetReleaseMeta(ontologyID string, version int) (*ReleaseSummary, error) {
	r, err := scanReleaseMeta(s.db.QueryRow(`SELECT version,version_name,spec_sha256,ttl_sha256,triples,published_at,manifest_json
		FROM ontology_release WHERE ontology_id=? AND version=?`, ontologyID, version))
	if errors.Is(err, sql.ErrNoRows) {
		return nil, ErrNotFound
	}
	if err != nil {
		return nil, err
	}
	r.OntologyID = ontologyID
	return r, nil
}

// ListReleases 本体全部发布快照元信息（version 正序=tag 目录；内容 blob 不取）。
func (s *Store) ListReleases(ontologyID string) ([]ReleaseSummary, error) {
	rows, err := s.db.Query(`SELECT version,version_name,spec_sha256,ttl_sha256,triples,published_at,manifest_json
		FROM ontology_release WHERE ontology_id=? ORDER BY version`, ontologyID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []ReleaseSummary{}
	for rows.Next() {
		r, err := scanReleaseMeta(rows)
		if err != nil {
			return nil, err
		}
		r.OntologyID = ontologyID
		out = append(out, *r)
	}
	return out, rows.Err()
}
