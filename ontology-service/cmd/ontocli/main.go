// ontocli 本体 CLI 第三消费面（REQ-438/M385，cobra 单二进制）。
//
// 定位=本体能力的命令行消费面（终端/CI 脚本/沙箱智能体 run_command 场景），从统一 REST
// 派生直调（构建平面 :8091 资产/spec 元素/导出 + 运行平面 :8090 方案/查询）——**非 REST/MCP
// 的包装底座**（REQ-436 接口统一批四方案对比 D 项明确否决：治理旁路/部署面膨胀/协议弱化）。
// 语义单源承诺：全部操作走与页面/MCP 同一套 REST 端点（元素级=REQ-436 上收语义），CLI 零本地
// 业务逻辑；输出=人读表格缺省 + --json 机读两态。
package main

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// 全局端点（--onto/--rt 覆盖；env ONTOCLI_ONTO / ONTOCLI_RT 优先级次之）。
var (
	ontoURL  = "http://127.0.0.1:8091"
	rtURL    = "http://127.0.0.1:8090"
	jsonOut  bool
	httpCli  = &http.Client{Timeout: 30 * time.Second}
	retErrSS = false
)

func main() {
	root := &cobra.Command{
		Use:   "ontocli",
		Short: "OntoAgents 本体 CLI——资产/spec 元素/运行方案/查询导出 命令行消费面（REQ-438）",
		Long: "本体能力的命令行消费面：从统一 REST 派生直调（构建平面资产与 spec 元素操作 + 运行平面方案与查询），\n" +
			"语义与页面/MCP 同源单管（REQ-436 上收语义）。非 REST/MCP 包装底座。\n" +
			"端点：--onto（默认 " + ontoURL + "）/ --rt（默认 " + rtURL + "），或 env ONTOCLI_ONTO / ONTOCLI_RT。",
		SilenceUsage:  true,
		SilenceErrors: true,
	}
	root.PersistentFlags().StringVar(&ontoURL, "onto", envOr("ONTOCLI_ONTO", "http://127.0.0.1:8091"), "构建平面地址（ontology-service）")
	root.PersistentFlags().StringVar(&rtURL, "rt", envOr("ONTOCLI_RT", "http://127.0.0.1:8090"), "运行平面地址（runtime-manager）")
	root.PersistentFlags().BoolVar(&jsonOut, "json", false, "机读 JSON 输出（缺省人读摘要）")

	root.AddCommand(assetCmd(), conceptCmd(), relationCmd(), planCmd(), sparqlCmd(), exportCmd())
	if err := root.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, "ontocli: "+err.Error())
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// ---- HTTP 薄客户端（直调 REST，零本地业务逻辑） ----

func call(base, method, path string, body any) ([]byte, int, error) {
	var rd io.Reader
	if body != nil {
		bts, err := json.Marshal(body)
		if err != nil {
			return nil, 0, err
		}
		rd = bytes.NewReader(bts)
	}
	req, err := http.NewRequest(method, strings.TrimRight(base, "/")+path, rd)
	if err != nil {
		return nil, 0, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := httpCli.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("%s 不可达: %w", base, err)
	}
	defer resp.Body.Close()
	bts, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, err
	}
	if resp.StatusCode >= 400 {
		var e struct {
			Error string `json:"error"`
		}
		_ = json.Unmarshal(bts, &e)
		if e.Error != "" {
			return bts, resp.StatusCode, fmt.Errorf("HTTP %d: %s", resp.StatusCode, e.Error)
		}
		return bts, resp.StatusCode, fmt.Errorf("HTTP %d", resp.StatusCode)
	}
	return bts, resp.StatusCode, nil
}

// emit 统一输出：--json 原样机读；缺省人读由调用方给摘要函数。
func emit(raw []byte, human func(raw []byte) string) {
	if jsonOut {
		var v any
		if json.Unmarshal(raw, &v) == nil {
			pretty, _ := json.MarshalIndent(v, "", "  ")
			fmt.Println(string(pretty))
			return
		}
	}
	if human != nil {
		fmt.Println(human(raw))
		return
	}
	fmt.Println(string(raw))
}

func printOK(msg string) {
	if !jsonOut {
		fmt.Println(msg)
	}
}

// ---- asset 资产组 ----

func assetCmd() *cobra.Command {
	c := &cobra.Command{Use: "asset", Short: "本体资产：list / get / create / rename / delete"}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出本体资产",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _, err := call(ontoURL, http.MethodGet, "/api/ontologies", nil)
			if err != nil {
				return err
			}
			emit(raw, func(b []byte) string {
				var os_ []struct {
					ID        string `json:"id"`
					Name      string `json:"name"`
					Version   int    `json:"version"`
					NConcepts int    `json:"n_concepts"`
				}
				_ = json.Unmarshal(b, &os_)
				var sb strings.Builder
				sb.WriteString("ID\t名称\t版本\t概念数\n")
				for _, o := range os_ {
					fmt.Fprintf(&sb, "%s\t%s\tv%d\t%d\n", o.ID, o.Name, o.Version, o.NConcepts)
				}
				return sb.String()
			})
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "get ID",
		Short: "查看本体元信息",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _, err := call(ontoURL, http.MethodGet, "/api/ontologies/"+args[0], nil)
			if err != nil {
				return err
			}
			emit(raw, nil)
			return nil
		},
	})
	create := &cobra.Command{
		Use:   "create NAME",
		Short: "创建本体（--desc 描述）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			desc, _ := cmd.Flags().GetString("desc")
			raw, _, err := call(ontoURL, http.MethodPost, "/api/ontologies", map[string]string{"name": args[0], "description": desc})
			if err != nil {
				return err
			}
			emit(raw, func(b []byte) string {
				var o struct {
					ID   string `json:"id"`
					Name string `json:"name"`
				}
				_ = json.Unmarshal(b, &o)
				return fmt.Sprintf("已创建 %s（%s）", o.Name, o.ID)
			})
			return nil
		},
	}
	create.Flags().String("desc", "", "本体描述")
	c.AddCommand(create)
	c.AddCommand(&cobra.Command{
		Use:   "rename ID NAME",
		Short: "重命名本体（PUT /api/ontologies/{id} 全量语义）",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			// 服务端 update=Name/Description 全量语义——先取旧值保 Description 不被清空
			old, _, err := call(ontoURL, http.MethodGet, "/api/ontologies/"+args[0], nil)
			if err != nil {
				return err
			}
			var o struct {
				Description string `json:"description"`
			}
			_ = json.Unmarshal(old, &o)
			if _, _, err := call(ontoURL, http.MethodPut, "/api/ontologies/"+args[0], map[string]string{"name": args[1], "description": o.Description}); err != nil {
				return err
			}
			printOK("已重命名 " + args[0] + " → " + args[1])
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "delete ID",
		Short: "删除本体（不可恢复）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := call(ontoURL, http.MethodDelete, "/api/ontologies/"+args[0], nil); err != nil {
				return err
			}
			printOK("已删除 " + args[0])
			return nil
		},
	})
	return c
}

// ---- concept / relation 元素组（REQ-436 元素级语义上收端点） ----

func conceptCmd() *cobra.Command {
	c := &cobra.Command{Use: "concept", Short: "概念元素：list / upsert / rename / delete（REQ-436 元素级端点）"}
	c.AddCommand(&cobra.Command{
		Use:   "list ONTO_ID",
		Short: "列出概念",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _, err := call(ontoURL, http.MethodGet, "/api/ontologies/"+args[0]+"/spec", nil)
			if err != nil {
				return err
			}
			emit(raw, func(b []byte) string {
				var sp struct {
					Concepts []struct {
						Name       string   `json:"name"`
						Label      string   `json:"label"`
						Definition string   `json:"definition"`
						Parents    []string `json:"parents"`
					} `json:"concepts"`
				}
				_ = json.Unmarshal(b, &sp)
				var sb strings.Builder
				sb.WriteString("名称\t标签\t上位\t定义\n")
				for _, x := range sp.Concepts {
					fmt.Fprintf(&sb, "%s\t%s\t%s\t%s\n", x.Name, x.Label, strings.Join(x.Parents, ","), x.Definition)
				}
				return sb.String()
			})
			return nil
		},
	})
	up := &cobra.Command{
		Use:   "upsert ONTO_ID NAME",
		Short: "按名创建/更新概念（--label --def --parents --synonyms）",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			label, _ := cmd.Flags().GetString("label")
			def, _ := cmd.Flags().GetString("def")
			parents, _ := cmd.Flags().GetString("parents")
			synonyms, _ := cmd.Flags().GetString("synonyms")
			body := map[string]any{"name": args[1]}
			if label != "" {
				body["label"] = label
			}
			if def != "" {
				body["definition"] = def
			}
			if parents != "" {
				body["parents"] = splitCSV(parents)
			}
			if synonyms != "" {
				body["synonyms"] = splitCSV(synonyms)
			}
			if _, _, err := call(ontoURL, http.MethodPost, "/api/ontologies/"+args[0]+"/elements/concept", body); err != nil {
				return err
			}
			printOK("已 upsert 概念 " + args[1])
			return nil
		},
	}
	up.Flags().String("label", "", "标签")
	up.Flags().String("def", "", "定义")
	up.Flags().String("parents", "", "上位概念（逗号分隔）")
	up.Flags().String("synonyms", "", "同义词（逗号分隔，REQ-419）")
	c.AddCommand(up)
	c.AddCommand(&cobra.Command{
		Use:   "rename ONTO_ID OLD NEW",
		Short: "概念重命名（引用改写随服务端 RewireConceptRefs）",
		Args:  cobra.ExactArgs(3),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := call(ontoURL, http.MethodPost, "/api/ontologies/"+args[0]+"/elements/concept/"+args[1]+"/rename", map[string]string{"to": args[2]}); err != nil {
				return err
			}
			printOK("已重命名 " + args[1] + " → " + args[2])
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "delete ONTO_ID NAME",
		Short: "删除概念",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := call(ontoURL, http.MethodDelete, "/api/ontologies/"+args[0]+"/elements/concept/"+args[1], nil); err != nil {
				return err
			}
			printOK("已删除概念 " + args[1])
			return nil
		},
	})
	return c
}

func relationCmd() *cobra.Command {
	c := &cobra.Command{Use: "relation", Short: "关系元素：list / upsert / delete"}
	c.AddCommand(&cobra.Command{
		Use:   "list ONTO_ID",
		Short: "列出关系",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _, err := call(ontoURL, http.MethodGet, "/api/ontologies/"+args[0]+"/spec", nil)
			if err != nil {
				return err
			}
			emit(raw, func(b []byte) string {
				var sp struct {
					Relations []struct {
						Name  string `json:"name"`
						Label string `json:"label"`
						From  string `json:"from"`
						To    string `json:"to"`
					} `json:"relations"`
				}
				_ = json.Unmarshal(b, &sp)
				var sb strings.Builder
				sb.WriteString("名称\t从\t到\n")
				for _, x := range sp.Relations {
					fmt.Fprintf(&sb, "%s\t%s\t%s\n", x.Name, x.From, x.To)
				}
				return sb.String()
			})
			return nil
		},
	})
	up := &cobra.Command{
		Use:   "upsert ONTO_ID NAME --from F --to T",
		Short: "按名创建/更新关系",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			from, _ := cmd.Flags().GetString("from")
			to, _ := cmd.Flags().GetString("to")
			label, _ := cmd.Flags().GetString("label")
			if from == "" || to == "" {
				return fmt.Errorf("--from 与 --to 必填")
			}
			body := map[string]any{"name": args[1], "from": from, "to": to}
			if label != "" {
				body["label"] = label
			}
			if _, _, err := call(ontoURL, http.MethodPost, "/api/ontologies/"+args[0]+"/elements/relation", body); err != nil {
				return err
			}
			printOK("已 upsert 关系 " + args[1])
			return nil
		},
	}
	up.Flags().String("from", "", "定义域概念名（必填）")
	up.Flags().String("to", "", "值域概念名（必填）")
	up.Flags().String("label", "", "标签")
	_ = up.MarkFlagRequired("from")
	_ = up.MarkFlagRequired("to")
	c.AddCommand(up)
	c.AddCommand(&cobra.Command{
		Use:   "delete ONTO_ID NAME",
		Short: "删除关系",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			if _, _, err := call(ontoURL, http.MethodDelete, "/api/ontologies/"+args[0]+"/elements/relation/"+args[1], nil); err != nil {
				return err
			}
			printOK("已删除关系 " + args[1])
			return nil
		},
	})
	return c
}

// ---- plan 运行方案组（运行平面 :8090） ----

func planCmd() *cobra.Command {
	c := &cobra.Command{Use: "plan", Short: "运行方案（runtime-manager）：list / status / start / stop"}
	c.AddCommand(&cobra.Command{
		Use:   "list",
		Short: "列出运行方案",
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _, err := call(rtURL, http.MethodGet, "/api/runtime-profiles", nil)
			if err != nil {
				return err
			}
			emit(raw, func(b []byte) string {
				var ps []struct {
					ID     string `json:"id"`
					Name   string `json:"name"`
					Status string `json:"status"`
				}
				_ = json.Unmarshal(b, &ps)
				var sb strings.Builder
				sb.WriteString("ID\t名称\t状态\n")
				for _, p := range ps {
					fmt.Fprintf(&sb, "%s\t%s\t%s\n", p.ID, p.Name, p.Status)
				}
				return sb.String()
			})
			return nil
		},
	})
	c.AddCommand(&cobra.Command{
		Use:   "status ID",
		Short: "查看方案",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _, err := call(rtURL, http.MethodGet, "/api/runtime-profiles/"+args[0], nil)
			if err != nil {
				return err
			}
			emit(raw, nil)
			return nil
		},
	})
	for _, act := range []struct{ name, verb string }{{"start", "启动"}, {"stop", "停止"}} {
		a := act
		c.AddCommand(&cobra.Command{
			Use:   a.name + " ID",
			Short: a.verb + "方案",
			Args:  cobra.ExactArgs(1),
			RunE: func(cmd *cobra.Command, args []string) error {
				if _, _, err := call(rtURL, http.MethodPost, "/api/runtime-profiles/"+args[0]+"/"+a.name, map[string]any{}); err != nil {
					return err
				}
				printOK("已" + a.verb + " " + args[0])
				return nil
			},
		})
	}
	return c
}

// ---- sparql 查询组 ----

func sparqlCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "sparql ONTO_ID \"QUERY\"",
		Short: "对运行方案执行 SPARQL 查询（POST /api/ontology-sparql）",
		Args:  cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			raw, _, err := call(rtURL, http.MethodPost, "/api/ontology-sparql", map[string]string{"ontology_id": args[0], "query": args[1]})
			if err != nil {
				return err
			}
			emit(raw, func(b []byte) string {
				var out struct {
					Bindings []map[string]any `json:"bindings"`
					Count    int              `json:"count"`
				}
				_ = json.Unmarshal(b, &out)
				return fmt.Sprintf("%d 行\n%v", out.Count, out.Bindings)
			})
			return nil
		},
	}
}

// ---- export 导出组（含 REQ-424 概念词典复用） ----

func exportCmd() *cobra.Command {
	var out string
	c := &cobra.Command{
		Use:   "export ONTO_ID --format FORMAT",
		Short: "导出本体（ttl | spec_json | dict_md | dict_html）",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			format, _ := cmd.Flags().GetString("format")
			switch format {
			case "ttl", "spec_json", "dict_md", "dict_html":
			default:
				return fmt.Errorf("不支持的 format %q（可选 ttl | spec_json | dict_md | dict_html）", format)
			}
			raw, _, err := call(ontoURL, http.MethodGet, "/api/ontologies/"+args[0]+"/export?format="+format, nil)
			if err != nil {
				return err
			}
			if out != "" {
				if werr := os.WriteFile(out, raw, 0o644); werr != nil {
					return werr
				}
				printOK(fmt.Sprintf("已导出 %s（%s，%d 字节）→ %s", args[0], format, len(raw), out))
				return nil
			}
			os.Stdout.Write(raw)
			fmt.Println()
			return nil
		},
	}
	c.Flags().String("format", "", "导出格式（必填）")
	c.Flags().StringVar(&out, "out", "", "写出文件路径（缺省打标准输出）")
	_ = c.MarkFlagRequired("format")
	return c
}

func splitCSV(s string) []string {
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if t := strings.TrimSpace(p); t != "" {
			out = append(out, t)
		}
	}
	return out
}
