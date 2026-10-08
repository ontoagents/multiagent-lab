package ontochat

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/xiaoyao/eino-multiagent-lab/ontology-service/internal/llmcreate"
)

func storySession() *Session {
	return &Session{ID: "sess-story", Stage: "story", Context: Context{Description: "测序数据质控本体"}}
}

func storyStore(t *testing.T) *Store {
	st := newJobTestStore(t)
	if _, err := st.Create("sess-story", "访谈测试"); err != nil {
		t.Fatal(err)
	}
	return st
}

// REQ-275：脚本化访谈——一问一轮/步进/跳过留痕；零 LLM 调用直至五问毕。
func TestStoryScriptedFlow(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("访谈轮不应调用 LLM")
		w.WriteHeader(500)
	}))
	defer srv.Close()
	// 注：summarizeStory 在第五问后才调用；该桩最后一刻换实现——改为计数后在第五问前不触发
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	st := storyStore(t)
	sess := storySession()

	for i := 0; i < 4; i++ {
		res, err := e.Turn(context.Background(), st, sess, "回答一", nil)
		if err != nil {
			t.Fatal(err)
		}
		if res.NextStage != "story" {
			t.Fatalf("阶段应保持 story: %s", res.NextStage)
		}
	}
	if sess.Context.StoryStep != 4 {
		t.Fatalf("四轮后应为第 4 步（0 起）: %d", sess.Context.StoryStep)
	}
	// 回退一步再答
	if _, err := e.StoryBack(st, sess); err != nil {
		t.Fatal(err)
	}
	if sess.Context.StoryStep != 3 {
		t.Fatalf("回退后应为 3: %d", sess.Context.StoryStep)
	}
}

// REQ-275：五问毕 one-shot 汇总（LLM）→ DraftStory + suggest；精修轮修订。
func TestStorySummarizeAndRevise(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/ontology-llm/generate", func(w http.ResponseWriter, r *http.Request) {
		b, _ := readAllBytes(r)
		if strings.Contains(b, "访谈记录") {
			_, _ = w.Write([]byte(`{"draft_json":"{\"story\":\"## 角色\\n测序实验员\",\"suggest\":\"补全层级关系\"}","usage":null}`))
			return
		}
		_, _ = w.Write([]byte(`{"draft_json":"{\"story\":\"## 角色\\n测序实验员（资深）\"}","usage":null}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	e := &Engine{LLM: llmcreate.New(srv.URL)}
	st := storyStore(t)
	sess := storySession()
	sess.Context.StoryAnswers = []string{"实验员小王", "沉淀质控知识", "手工整理易错", "FASTQ 报告", "自动问答"}
	sess.Context.StoryStep = len(StoryQuestions)

	res, err := e.Turn(context.Background(), st, sess, "回答五", nil)
	if err != nil {
		t.Fatal(err)
	}
	if sess.Context.DraftStory == "" || !strings.Contains(sess.Context.DraftStory, "测序实验员") {
		t.Fatalf("汇总未落 DraftStory: %q", sess.Context.DraftStory)
	}
	if !strings.Contains(res.Reply, "用户故事已生成") {
		t.Fatalf("回复形态不符: %.80s", res.Reply)
	}
	// 精修
	res2, err := e.Turn(context.Background(), st, sess, "把角色写得更资深一些", nil)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(sess.Context.DraftStory, "资深") || !strings.Contains(res2.Reply, "已按意见修订") {
		t.Fatalf("精修未生效: %q / %.60s", sess.Context.DraftStory, res2.Reply)
	}
}

// REQ-275：故事材料优先进 CQ 抽取。
func TestBuildCQMaterialStoryFirst(t *testing.T) {
	sess := storySession()
	sess.Context.DraftStory = "## 角色\n实验员"
	m := buildCQMaterial(sess)
	if !strings.HasPrefix(m, "用户故事（访谈制品）：") || !strings.Contains(m, "测序数据质控本体") {
		t.Fatalf("故事材料应优先: %.60s", m)
	}
}

func readAllBytes(r *http.Request) (string, error) {
	buf := make([]byte, 0, 256<<10)
	tmp := make([]byte, 64<<10)
	for {
		n, err := r.Body.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf), nil
}
