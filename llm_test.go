package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

// fakeLLM answers like an OpenAI-compatible server (Ollama, LM Studio, llama.cpp).
func fakeLLM(t *testing.T, reply func(prompt string) string) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string } `json:"messages"`
		}
		json.NewDecoder(r.Body).Decode(&req)
		last := req.Messages[len(req.Messages)-1].Content
		json.NewEncoder(w).Encode(map[string]interface{}{
			"choices": []map[string]interface{}{{"message": map[string]string{"content": reply(last)}}},
		})
	}))
}

func testDB(t *testing.T) {
	t.Setenv("SOCRATEAN_DB", filepath.Join(t.TempDir(), "t.db"))
	t.Setenv("ANTHROPIC_API_KEY", "")
	t.Setenv("SOCRATEAN_PROVIDER", "")
	var err error
	db, err = initDB()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { db.Close() })
	initSchool()
	detectedLLM = LLMConfig{}
}

func TestOpenAICompleteStripsThinking(t *testing.T) {
	srv := fakeLLM(t, func(string) string { return "<think>hmm\nlet me see</think>\nWhat do you mean by scarcity?" })
	defer srv.Close()
	got, err := openAIComplete(srv.URL, "", "m", "sys", "hi", 0)
	if err != nil || got != "What do you mean by scarcity?" {
		t.Fatalf("got %q, %v", got, err)
	}
	if _, err := openAIComplete("http://127.0.0.1:1", "", "m", "", "hi", 0); err == nil {
		t.Fatal("unreachable server should error")
	}
}

func TestProviderConfigAndJudge(t *testing.T) {
	testDB(t)
	h := NewAPIHandler("")
	if h.aiOn() {
		t.Fatal("AI should be off with nothing configured")
	}

	student := "Higher prices make me buy less because other things become a better deal for my money."
	srv := fakeLLM(t, func(prompt string) string {
		// a model that cites a real quote
		return "YES\nQUOTE: Higher prices make me buy less"
	})
	defer srv.Close()

	cfg, _ := json.Marshal(LLMConfig{Provider: "openai", URL: srv.URL, Model: "tutor-m"})
	db.Exec(`UPDATE school SET llm = ? WHERE id = 1`, string(cfg))
	c, src := resolveLLM()
	if src != "saved" || c.JudgeModel != "tutor-m" || !h.aiOn() {
		t.Fatalf("config not picked up: %+v %s", c, src)
	}

	found, method := h.judgeObjectives("Econ", []string{"Explain demand"}, nil, student)
	if method != "ai" || found["Explain demand"] != "Higher prices make me buy less" {
		t.Fatalf("judge result: %v %s", found, method)
	}

	// a model that invents a quote must not earn the student credit
	lying := fakeLLM(t, func(string) string {
		return `[{"n": 1, "quote": "Demand slopes down due to the law of marginal utility"}]`
	})
	defer lying.Close()
	cfg, _ = json.Marshal(LLMConfig{Provider: "openai", URL: lying.URL, Model: "m"})
	db.Exec(`UPDATE school SET llm = ? WHERE id = 1`, string(cfg))
	if found, _ := h.judgeObjectives("Econ", []string{"Explain demand"}, nil, student); len(found) != 0 {
		t.Fatalf("fabricated quote earned credit: %v", found)
	}
}

func TestDetectedAndEnvPrecedence(t *testing.T) {
	testDB(t)
	detectedLLM = LLMConfig{Provider: "openai", URL: ollamaURL, Model: "llama3.2:latest"}
	if c, src := resolveLLM(); src != "detected" || c.Model != "llama3.2:latest" {
		t.Fatalf("detected: %+v %s", c, src)
	}
	t.Setenv("SOCRATEAN_PROVIDER", "openai")
	t.Setenv("SOCRATEAN_MODEL", "from-env")
	if c, src := resolveLLM(); src != "env" || c.Model != "from-env" {
		t.Fatalf("env should beat detection: %+v %s", c, src)
	}
	db.Exec(`UPDATE school SET llm = '{"provider":"openai","model":"from-admin","judge_model":"bigger"}' WHERE id = 1`)
	if c, src := resolveLLM(); src != "saved" || c.Model != "from-admin" || c.JudgeModel != "bigger" {
		t.Fatalf("admin choice should win: %+v %s", c, src)
	}
}

func TestShortlistAndJudgeEach(t *testing.T) {
	open := []string{"Explain why supply curves slope upward", "Explain price elasticity of demand", "Describe market equilibrium", "Define opportunity cost"}
	got := shortlistObjectives(open, "elastic demand means buyers react a lot to a price change", 4)
	if len(got) == 0 || got[0] != "Explain price elasticity of demand" {
		t.Fatalf("shortlist: %v", got)
	}
	if none := shortlistObjectives(open, "hello there", 4); len(none) != 2 || none[0] != open[0] {
		t.Fatalf("fallback shortlist: %v", none)
	}

	testDB(t)
	student := "People react a lot when the price of a luxury changes, but barely at all for medicine."
	srv := fakeLLM(t, func(p string) string {
		return "YES\nQUOTE: \"People react a lot when the price of a luxury changes\""
	})
	defer srv.Close()
	cfg, _ := json.Marshal(LLMConfig{Provider: "openai", URL: srv.URL, Model: "m"})
	db.Exec(`UPDATE school SET llm = ? WHERE id = 1`, string(cfg))
	h := NewAPIHandler("")
	found, _ := h.judgeObjectives("Econ", open, nil, student)
	if found["Explain price elasticity of demand"] != "People react a lot when the price of a luxury changes" {
		t.Fatalf("judgeEach: %v", found)
	}
}

func TestSavedKeyIsSentToProvider(t *testing.T) {
	testDB(t)
	var auth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		auth = r.Header.Get("Authorization")
		json.NewEncoder(w).Encode(map[string]interface{}{"choices": []map[string]interface{}{{"message": map[string]string{"content": "ok"}}}})
	}))
	defer srv.Close()
	h := NewAPIHandler("")
	cfg, _ := json.Marshal(LLMConfig{Provider: "openai", URL: srv.URL, Model: "m"})
	db.Exec(`UPDATE school SET llm = ?, llm_key = 'sk-test' WHERE id = 1`, string(cfg))
	if _, err := h.ai("tutor", "", "hi"); err != nil || auth != "Bearer sk-test" {
		t.Fatalf("saved key not sent: %q %v", auth, err)
	}
	if st := h.llmStatus(); st["key_saved"] != true {
		t.Fatal("status should say a key is saved")
	}
	if b, _ := json.Marshal(h.llmStatus()); strings.Contains(string(b), "sk-test") {
		t.Fatal("status must never contain the key")
	}
}
