package evaldata

// REQ-345/M145：CQ 基准集一致性锁定——数据集文件（seeds/evaldata/ontology-cq/，gold standard 单源）
// 与种子 spec.CQ（internal/seed/examples，embed 进构建平面）双端同步；gold terms 必须在种子
// 概念/关系中实际承载（CQ 可答性的结构性前提——81 号 CQ4OE gold standard 方法论）。
// 本测试无 build tag（始终运行）：基准集是产品资产，漂移即缺陷。

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	pkgspec "github.com/xiaoyao/eino-multiagent-lab/pkg/ontology/spec"
)

type cqDataset struct {
	Note     string `json:"note"`
	Ontology string `json:"ontology"`
	CQs      []struct {
		ID   string   `json:"id"`
		CQ   string   `json:"cq"`
		Gold []string `json:"gold"`
		Type string   `json:"type"`
	} `json:"cqs"`
}

const datasetDir = "../../../seeds/evaldata/ontology-cq"

func TestCQDatasetConsistency(t *testing.T) {
	ents, err := os.ReadDir(datasetDir)
	if err != nil {
		t.Skipf("数据集目录不可达（%v）——仓库形态随迁，构建平面内嵌路径不含", err)
	}
	checked := 0
	for _, e := range ents {
		if !strings.HasSuffix(e.Name(), ".json") {
			continue
		}
		raw, rerr := os.ReadFile(filepath.Join(datasetDir, e.Name()))
		if rerr != nil {
			t.Fatalf("%s 读取失败: %v", e.Name(), rerr)
		}
		var ds cqDataset
		if err := json.Unmarshal(raw, &ds); err != nil {
			t.Fatalf("%s 解析失败: %v", e.Name(), err)
		}
		if len(ds.CQs) == 0 {
			t.Fatalf("%s 基准集为空", e.Name())
		}
		// ① 种子 spec 存在且 CQ 与数据集逐条一致（双端同步）
		seedRaw, serr := os.ReadFile("../../internal/seed/examples/" + ds.Ontology + ".json")
		if serr != nil {
			t.Fatalf("种子 %s 读取失败: %v", ds.Ontology, serr)
		}
		var sp pkgspec.Spec
		if err := json.Unmarshal(seedRaw, &sp); err != nil {
			t.Fatalf("种子 %s 解析失败: %v", ds.Ontology, err)
		}
		if len(sp.CQ) != len(ds.CQs) {
			t.Fatalf("%s: 种子 spec.CQ %d 条 ≠ 数据集 %d 条（双端漂移）", ds.Ontology, len(sp.CQ), len(ds.CQs))
		}
		for i, dc := range ds.CQs {
			if sp.CQ[i] != dc.CQ {
				t.Fatalf("%s: 第 %d 条 CQ 漂移\n  数据集: %s\n  种子:   %s", ds.Ontology, i+1, dc.CQ, sp.CQ[i])
			}
			if len(dc.Gold) == 0 {
				t.Fatalf("%s: %s 缺 gold terms", ds.Ontology, dc.ID)
			}
		}
		// ② gold terms 在种子概念/关系中实际承载（可答性结构性前提）
		names := map[string]bool{}
		for _, c := range sp.Concepts {
			names[c.Name] = true
		}
		for _, r := range sp.Relations {
			names[r.Name] = true
		}
		for _, dc := range ds.CQs {
			for _, g := range dc.Gold {
				if !names[g] {
					t.Fatalf("%s: %s gold term %q 未在种子概念/关系中承载（CQ 不可答）", ds.Ontology, dc.ID, g)
				}
			}
		}
		checked++
	}
	if checked < 5 {
		t.Fatalf("基准集应覆盖 ≥5 份种子，实际 %d", checked)
	}
}
