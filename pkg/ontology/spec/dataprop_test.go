package spec

import "testing"

// REQ-268/M77：数据属性声明校验——name 必填唯一、domain 引用已定义概念；
// range 宽松不校验（未知类型诚实保留）；实例属性键未声明不在此阻断（qualitygate info 级承载，兼容存量资产）。
func TestValidateDataProperties(t *testing.T) {
	ok := &Spec{
		Name:           "t",
		Concepts:       []Concept{{Name: "Pod"}},
		DataProperties: []DataProperty{{Name: "副本数", Domain: "Pod", Range: "integer"}},
		Instances:      []Instance{{Name: "p1", Concept: "Pod", Attributes: map[string]any{"未声明键": "x"}}},
	}
	if errs := ok.Validate(); len(errs) != 0 {
		t.Fatalf("合法声明+未声明实例键不应报错（qualitygate info 承载）: %v", errs)
	}

	bad := &Spec{
		Name:     "t",
		Concepts: []Concept{{Name: "Pod"}},
		DataProperties: []DataProperty{
			{Name: "副本数", Domain: "不存在的概念"},
			{Name: ""},
			{Name: "副本数"},
		},
	}
	errs := bad.Validate()
	if len(errs) != 3 {
		t.Fatalf("期望 3 条错误（domain 悬空/空名/重名），实际 %d: %v", len(errs), errs)
	}
	paths := map[string]bool{}
	for _, e := range errs {
		paths[e.Path] = true
	}
	for _, want := range []string{"data_properties[0].domain", "data_properties[1].name", "data_properties[2].name"} {
		if !paths[want] {
			t.Fatalf("缺少错误路径 %s: %v", want, errs)
		}
	}
}
