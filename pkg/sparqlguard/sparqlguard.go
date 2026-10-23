// Package sparqlguard SPARQL 只读白名单单一实现（REQ-380/M176 收口）。
//
// 此前四处词法副本各自维护（facade vetReadonlySelect / cqbench VetReadonlySelect〔自注
// 「服务化副本」〕/ rest_nl2sparql vetReadonlySelect〔自注「同口径本地实现」〕/ toolchain
// VetReadonlySelect），口径已现漂移（facade 支持 ASK〔REQ-366/T1〕，其余仅 SELECT）。
// 本包为唯一事实源：PREFIX/BASE 前导声明后首关键词须为 SELECT 或 ASK（统一最宽只读口径），
// 全文裸词不得命中变更关键字（字符串字面量、IRI、注释中的同形词不误伤）。
// 解析器级校验为观察项不在本包（词法级=轻量预检，执行侧引擎/代理为权威兜底）。
package sparqlguard

import (
	"fmt"
	"strings"
	"unicode"
)

// updateKeywords SPARQL Update 变更关键字黑名单（SPARQL 1.1 Update 词表收敛）。
var updateKeywords = map[string]bool{
	"LOAD": true, "CLEAR": true, "CREATE": true, "DROP": true,
	"MOVE": true, "COPY": true, "ADD": true,
	"INSERT": true, "DELETE": true, "UPDATE": true,
}

// IsUpdateKeyword 裸词是否命中变更关键字（大写归一后比对）。
func IsUpdateKeyword(word string) bool {
	return updateKeywords[strings.ToUpper(word)]
}

// Tokens 词法扫描：收集裸词 token；字符串字面量（含 ''"/"""" 长串与反斜杠转义）、
// IRI <…>、注释 #… 的内容不产出 token（facade sparqlTokens 同实现，本包为准）。
func Tokens(q string) []string {
	var toks []string
	var cur strings.Builder
	flush := func() {
		if cur.Len() > 0 {
			toks = append(toks, cur.String())
			cur.Reset()
		}
	}
	r := []rune(q)
	n := len(r)
	for i := 0; i < n; i++ {
		c := r[i]
		switch {
		case c == '#':
			flush()
			for i < n && r[i] != '\n' {
				i++
			}
		case c == '"' || c == '\'':
			flush()
			if i+2 < n && r[i+1] == c && r[i+2] == c {
				i += 3
				for i < n && !(r[i] == c && i+2 < n && r[i+1] == c && r[i+2] == c) {
					if r[i] == '\\' {
						i++
					}
					i++
				}
				i += 2
			} else {
				i++
				for i < n && r[i] != c {
					if r[i] == '\\' {
						i++
					}
					i++
				}
			}
		case c == '<':
			flush()
			for i < n && r[i] != '>' {
				i++
			}
		case unicode.IsLetter(c) || unicode.IsDigit(c) || c == '_':
			cur.WriteRune(c)
		default:
			flush()
		}
	}
	flush()
	return toks
}

// FirstQueryKind 前导 PREFIX/BASE 声明后的首关键词（SELECT/ASK 判别用；无=空串）。
func FirstQueryKind(query string) string {
	tokens := Tokens(query)
	i := 0
	for i < len(tokens) {
		t := strings.ToUpper(tokens[i])
		if t == "PREFIX" {
			i += 2 // 前缀名 + 被词法跳过的 <IRI>
		} else if t == "BASE" {
			i++ // 被词法跳过的 <IRI>
		} else {
			return t
		}
	}
	return ""
}

// Vet 只读白名单校验：PREFIX/BASE 前导声明后首关键词必须是 SELECT 或 ASK
// （统一口径——facade 最宽只读面为基准，cqbench/nl2sparql/toolchain 三处窄副本随收口放宽；
// ASK 为 SPARQL 1.1 只读问句形态，REQ-366/T1 拍板），全文裸词不得命中变更关键字。
func Vet(query string) error {
	tokens := Tokens(query)
	if len(tokens) == 0 {
		return fmt.Errorf("空查询")
	}
	head := FirstQueryKind(query)
	if head != "SELECT" && head != "ASK" {
		i := 0
		for i < len(tokens) {
			t := strings.ToUpper(tokens[i])
			if t == "PREFIX" {
				i += 2
			} else if t == "BASE" {
				i++
			} else {
				break
			}
		}
		return fmt.Errorf("仅允许 SELECT/ASK 查询，得到首关键词 %q（REQ-151 只读白名单）", tokens[min(i, len(tokens)-1)])
	}
	for _, t := range tokens {
		if IsUpdateKeyword(t) {
			return fmt.Errorf("含变更关键字 %q，已拒绝（REQ-151 只读白名单）", t)
		}
	}
	return nil
}
