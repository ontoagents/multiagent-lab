package facade

// REQ-415/M211：questionWords 确定性分词单测（CJK bigram/ASCII 词/混合/单字）。
import "testing"

func TestQuestionWords(t *testing.T) {
	cases := []struct {
		name, q string
		want    []string
	}{
		{"中文 bigram", "有哪些计算节点", []string{"有哪", "哪些", "些计", "计算", "算节", "节点"}},
		{"ASCII 词", "what is Pod", []string{"what", "is", "pod"}},
		{"混合", "查 Pod 驱逐", []string{"查", "pod", "驱逐"}},
		{"单字", "查 节点", []string{"查", "节点"}},
	}
	for _, c := range cases {
		got := questionWords(c.q)
		if len(got) == 0 {
			t.Fatalf("%s: 空结果", c.name)
		}
		for _, w := range c.want {
			found := false
			for _, g := range got {
				if g == w {
					found = true
					break
				}
			}
			if !found {
				t.Errorf("%s: 缺词 %q（got %v）", c.name, w, got)
			}
		}
	}
}
