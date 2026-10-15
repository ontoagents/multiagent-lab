package importer

// REQ-325 windows 计数标记单测：sidecar stderr CRLF 形态（python on windows 输出 \r\n）
// 也须解析出 TRIPLES 计数——原 (?m)$ 不含 \r 前形态，计数恒丢 → X-Onto-Triples 头恒缺 →
// 装载对账硬闸（REQ-293①）在 windows 全线 skipped。

import (
	"strconv"
	"testing"
)

func TestTriplesReCRLF(t *testing.T) {
	cases := map[string]int{
		"TRIPLES 97\r\n":             97, // windows CRLF
		"TRIPLES 97\n":               97, // unix LF
		"noise\nTRIPLES 3\r\nmore\n": 3,  // 多行噪声中提取
		"TRIPLES x\r\n":              -1, // 非数字不命中
		"":                           -1, // 空输入不命中
	}
	for in, want := range cases {
		m := triplesRe.FindStringSubmatch(in)
		if want < 0 {
			if m != nil {
				t.Fatalf("%q 不应命中，得 %v", in, m)
			}
			continue
		}
		if m == nil {
			t.Fatalf("%q 应命中计数 %d", in, want)
		}
		if m[1] != strconv.Itoa(want) {
			t.Fatalf("%q 计数应 %d，得 %s", in, want, m[1])
		}
	}
}
