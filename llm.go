package main

// The AI behind the tutor is swappable. A school can run everything on its own machine with
// Ollama (or LM Studio, llama.cpp, vLLM: anything that speaks the OpenAI chat API), or use
// Anthropic's cloud with a key. Student text only leaves the building if the school chooses
// a cloud provider. A key pasted into the admin panel lives in the school's own database file
// (school.llm_key) and is never sent back to the browser; an environment variable still works.

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

type LLMConfig struct {
	Provider   string `json:"provider"`    // "anthropic", "openai" (OpenAI-compatible, e.g. Ollama), or "none"
	URL        string `json:"url"`         // base URL for "openai", e.g. http://localhost:11434/v1
	Model      string `json:"model"`       // the tutor model
	JudgeModel string `json:"judge_model"` // grades understanding and reads syllabi; defaults to Model
}

const ollamaURL = "http://localhost:11434/v1"

// detectedLLM is filled at startup when nothing is configured but a local Ollama is running.
var detectedLLM LLMConfig

func envLLM() LLMConfig {
	c := LLMConfig{
		Provider:   os.Getenv("SOCRATEAN_PROVIDER"),
		URL:        os.Getenv("SOCRATEAN_LLM_URL"),
		Model:      os.Getenv("SOCRATEAN_MODEL"),
		JudgeModel: os.Getenv("SOCRATEAN_JUDGE_MODEL"),
	}
	if c.Provider == "" && os.Getenv("ANTHROPIC_API_KEY") != "" {
		c.Provider = "anthropic"
	}
	return c
}

// resolveLLM returns the active configuration and where it came from:
// "saved" (set by the school admin), "env", "detected" (local Ollama found), or "none".
func resolveLLM() (LLMConfig, string) {
	var raw string
	db.QueryRow(`SELECT COALESCE(llm, '{}') FROM school WHERE id = 1`).Scan(&raw)
	var saved LLMConfig
	json.Unmarshal([]byte(raw), &saved)
	if saved.Provider != "" {
		return normalizeLLM(saved), "saved"
	}
	if e := envLLM(); e.Provider != "" {
		return normalizeLLM(e), "env"
	}
	if detectedLLM.Provider != "" {
		return normalizeLLM(detectedLLM), "detected"
	}
	return LLMConfig{Provider: "none"}, "none"
}

func normalizeLLM(c LLMConfig) LLMConfig {
	c.Provider = strings.ToLower(strings.TrimSpace(c.Provider))
	switch c.Provider {
	case "anthropic":
		if c.Model == "" {
			c.Model = claudeModel()
		}
	case "openai":
		if c.URL == "" {
			c.URL = ollamaURL
		}
		c.URL = strings.TrimRight(c.URL, "/")
	default:
		return LLMConfig{Provider: "none"}
	}
	if c.JudgeModel == "" {
		c.JudgeModel = c.Model
	}
	return c
}

// savedKey is the key the admin pasted into the panel ("" if none).
func savedKey() string {
	var k string
	db.QueryRow(`SELECT COALESCE(llm_key, '') FROM school WHERE id = 1`).Scan(&k)
	return k
}

// apiKeyFor picks the saved key first, then the environment.
func (h *APIHandler) apiKeyFor(provider string) string {
	if k := savedKey(); k != "" {
		return k
	}
	if provider == "anthropic" {
		return h.apiKey
	}
	return os.Getenv("SOCRATEAN_LLM_KEY")
}

func (h *APIHandler) aiOn() bool {
	c, _ := resolveLLM()
	if c.Provider == "anthropic" {
		return h.apiKeyFor("anthropic") != ""
	}
	return c.Provider == "openai" && c.Model != ""
}

// ai makes one completion. role is "tutor" or "judge" and picks the model.
func (h *APIHandler) ai(role, system, prompt string) (string, error) {
	c, _ := resolveLLM()
	model := c.Model
	if role == "judge" {
		model = c.JudgeModel
	}
	switch c.Provider {
	case "anthropic":
		return NewClaudeClient(h.apiKeyFor("anthropic")).CompleteModel(model, prompt, system)
	case "openai":
		temp := 0.4 // the tutor can vary its questions
		if role == "judge" {
			temp = 0 // grading should give the same answer every time
		}
		return openAIComplete(c.URL, h.apiKeyFor("openai"), model, system, prompt, temp)
	}
	return "", fmt.Errorf("no AI model is configured")
}

var thinkRe = regexp.MustCompile(`(?s)<think>.*?</think>`)

// openAIComplete speaks the OpenAI chat completions API, which Ollama and most local servers provide.
func openAIComplete(baseURL, key, model, system, prompt string, temperature float64) (string, error) {
	msgs := []map[string]string{}
	if system != "" {
		msgs = append(msgs, map[string]string{"role": "system", "content": system})
	}
	msgs = append(msgs, map[string]string{"role": "user", "content": prompt})
	body, _ := json.Marshal(map[string]interface{}{
		"model": model, "messages": msgs, "max_tokens": 1024, "temperature": temperature, "stream": false,
	})
	req, err := http.NewRequest("POST", baseURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	if key != "" {
		req.Header.Set("Authorization", "Bearer "+key)
	}
	resp, err := (&http.Client{Timeout: 180 * time.Second}).Do(req)
	if err != nil {
		return "", fmt.Errorf("could not reach the AI server at %s: %v", baseURL, err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return "", fmt.Errorf("AI server error (%d): %s", resp.StatusCode, truncate(string(raw), 300))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || len(out.Choices) == 0 {
		return "", fmt.Errorf("unexpected reply from the AI server")
	}
	return strings.TrimSpace(thinkRe.ReplaceAllString(out.Choices[0].Message.Content, "")), nil
}

// listModels asks a local server which models it has. Ollama's native list first, then the OpenAI one.
func listModels(baseURL string) []string {
	baseURL = strings.TrimRight(baseURL, "/")
	client := &http.Client{Timeout: 4 * time.Second}
	var out []string
	if strings.HasSuffix(baseURL, "/v1") {
		if resp, err := client.Get(strings.TrimSuffix(baseURL, "/v1") + "/api/tags"); err == nil {
			defer resp.Body.Close()
			var t struct {
				Models []struct {
					Name string `json:"name"`
				} `json:"models"`
			}
			if json.NewDecoder(resp.Body).Decode(&t) == nil {
				for _, m := range t.Models {
					out = append(out, m.Name)
				}
			}
			if len(out) > 0 {
				return out
			}
		}
	}
	if resp, err := client.Get(baseURL + "/models"); err == nil {
		defer resp.Body.Close()
		var t struct {
			Data []struct {
				ID string `json:"id"`
			} `json:"data"`
		}
		if json.NewDecoder(resp.Body).Decode(&t) == nil {
			for _, m := range t.Data {
				out = append(out, m.ID)
			}
		}
	}
	return out
}

// detectLocalLLM is called at startup. If nothing is configured and Ollama is running, use it.
func detectLocalLLM() {
	if c, src := resolveLLM(); src != "none" && c.Provider != "none" {
		return
	}
	models := listModels(ollamaURL)
	if len(models) > 0 {
		detectedLLM = LLMConfig{Provider: "openai", URL: ollamaURL, Model: models[0]}
	}
}

// ==================== ADMIN: AI MODEL SETTINGS ====================

func (h *APIHandler) llmStatus() map[string]interface{} {
	c, src := resolveLLM()
	return map[string]interface{}{
		"config": c, "source": src, "active": h.aiOn(),
		"key_saved": savedKey() != "",
		"key_env":   h.apiKey != "" || os.Getenv("SOCRATEAN_LLM_KEY") != "",
		"models":    localModels(c),
	}
}

func localModels(c LLMConfig) []string {
	url := c.URL
	if url == "" {
		url = ollamaURL
	}
	if c.Provider == "anthropic" {
		return []string{}
	}
	m := listModels(url)
	if m == nil {
		m = []string{}
	}
	return m
}

// AI status for any signed-in teacher, so the panel can warn when no model is connected.
func (h *APIHandler) AIStatus(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "teacher", "admin"); !ok {
		return
	}
	c, src := resolveLLM()
	writeJSON(w, map[string]interface{}{"active": h.aiOn(), "provider": c.Provider, "model": c.Model, "judge_model": c.JudgeModel, "source": src})
}

func (h *APIHandler) AdminLLM(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "admin"); !ok {
		return
	}
	if r.Method == "POST" {
		var req struct {
			LLMConfig
			APIKey   string `json:"api_key"`
			ClearKey bool   `json:"clear_key"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		if k := strings.TrimSpace(req.APIKey); k != "" {
			db.Exec(`UPDATE school SET llm_key = ? WHERE id = 1`, k)
		} else if req.ClearKey {
			db.Exec(`UPDATE school SET llm_key = '' WHERE id = 1`)
		}
		cfg := normalizeLLM(req.LLMConfig)
		// A key belongs to the provider it was pasted for. Pointing at a different one drops it,
		// so a key is never sent to a server the admin did not give it to.
		if old, src := resolveLLM(); src == "saved" && strings.TrimSpace(req.APIKey) == "" &&
			(old.Provider != cfg.Provider || old.URL != cfg.URL) {
			db.Exec(`UPDATE school SET llm_key = '' WHERE id = 1`)
		}
		if cfg.Provider == "none" {
			cfg = LLMConfig{} // clears the saved choice, falling back to env or detection
		}
		b, _ := json.Marshal(cfg)
		db.Exec(`UPDATE school SET llm = ? WHERE id = 1`, string(b))
	}
	writeJSON(w, h.llmStatus())
}

func (h *APIHandler) AdminLLMModels(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "admin"); !ok {
		return
	}
	url := r.URL.Query().Get("url")
	if url == "" {
		url = ollamaURL
	}
	m := listModels(url)
	if m == nil {
		m = []string{}
	}
	writeJSON(w, m)
}

func (h *APIHandler) AdminLLMTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "admin"); !ok {
		return
	}
	start := time.Now()
	reply, err := h.ai("tutor", "You are a test. Reply with one short sentence.", "Say that you are ready to help a student think.")
	res := map[string]interface{}{"ok": err == nil, "ms": time.Since(start).Milliseconds()}
	if err != nil {
		res["error"] = err.Error()
	} else {
		res["reply"] = truncate(reply, 200)
	}
	writeJSON(w, res)
}

// ==================== GRADING SELF-TEST ====================

type gradeCase struct {
	Name       string
	Message    string
	WantCredit bool
}

var gradeObjective = "Explain why demand curves slope downward."

var gradeCases = []gradeCase{
	{"Explains in own words", "When the price of something goes up, I buy less of it because other things become the better deal for my money, and when it drops I'm happy to buy more. Lots of people doing that is why the line goes down.", true},
	{"Applies to a new case", "Think of concert tickets: if they cost double, a lot of people would skip the show and spend it elsewhere, so fewer tickets sell. That's the downward slope.", true},
	{"Just repeats the wording", "Demand curves slope downward. I would explain why demand curves slope downward.", false},
	{"Vague agreement", "Yeah that makes sense, I think I get it now.", false},
	{"Wrong explanation", "Demand curves slope downward because when prices are higher, people want the product more and buy more of it.", false},
}

// AdminLLMSelfTest runs fixed student answers through the grader so a school can see whether
// the model they chose can really tell understanding from parroting.
func (h *APIHandler) AdminLLMSelfTest(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "admin"); !ok {
		return
	}
	if !h.aiOn() {
		http.Error(w, "No AI model is connected yet.", http.StatusConflict)
		return
	}
	type result struct {
		Name   string `json:"name"`
		Want   bool   `json:"want_credit"`
		Got    bool   `json:"got_credit"`
		Method string `json:"method"`
		Pass   bool   `json:"pass"`
	}
	results := []result{}
	passed := 0
	for _, gc := range gradeCases {
		found, method := h.judgeObjectives("Economics", []string{gradeObjective}, nil, gc.Message)
		_, got := found[gradeObjective]
		pass := method != "error" && got == gc.WantCredit
		if pass {
			passed++
		}
		results = append(results, result{gc.Name, gc.WantCredit, got, method, pass})
	}
	verdict := "Not reliable enough to grade students. Pick a larger model, or set a stronger judge model."
	if passed == len(gradeCases) {
		verdict = "Grades these examples correctly."
	} else if passed == len(gradeCases)-1 {
		verdict = "Mostly reliable. Read the evidence quotes before trusting a grade."
	}
	writeJSON(w, map[string]interface{}{"passed": passed, "total": len(gradeCases), "verdict": verdict, "results": results})
}

// ==================== GRADING WITH SMALL LOCAL MODELS ====================

var wordRe = regexp.MustCompile(`[a-z]{4,}`)

func stems(s string) map[string]bool {
	out := map[string]bool{}
	for _, w := range wordRe.FindAllString(strings.ToLower(s), -1) {
		if len(w) > 6 {
			w = w[:6] // crude stem so "elastic" matches "elasticity"
		}
		out[w] = true
	}
	return out
}

// shortlistObjectives picks the open objectives a message most likely speaks to, so a small
// model is asked about a few, not all. With no overlap it falls back to the first two.
func shortlistObjectives(open []string, message string, max int) []string {
	msg := stems(message)
	type scored struct {
		name  string
		score int
		pos   int
	}
	var all []scored
	for i, o := range open {
		n := 0
		for w := range stems(o) {
			if msg[w] {
				n++
			}
		}
		all = append(all, scored{o, n, i})
	}
	var out []string
	for len(out) < max {
		best := -1
		for i, a := range all {
			if a.score > 0 && (best < 0 || a.score > all[best].score) {
				best = i
			}
		}
		if best < 0 {
			break
		}
		out = append(out, all[best].name)
		all[best].score = -1
	}
	for _, a := range all {
		if len(out) >= 2 {
			break
		}
		if a.score >= 0 {
			out = append(out, a.name)
			all[a.pos].score = -1
		}
	}
	return out
}

// judgeEach asks one simple YES/NO question per objective. Small models follow this reliably
// where they stumble on numbering several objectives at once. The quote must still be verbatim.
func (h *APIHandler) judgeEach(subject string, open []string, message string) map[string]string {
	found := map[string]string{}
	sys := "You are a strict grader of student understanding."
	for _, obj := range shortlistObjectives(open, message, 4) {
		prompt := fmt.Sprintf(`A student is learning (%s): %s

The student wrote:
"""%s"""

Does the student explain this idea in their own words, apply it to a new case, or correctly fix a misconception? Answer NO if they only repeat the wording, agree vaguely, guess, or explain it wrongly.

Reply with YES or NO on the first line. If YES, add a second line that starts with QUOTE: followed by the exact words copied from the student's message that show it.`, subject, obj, message)
		resp, err := h.ai("judge", sys, prompt)
		if err != nil {
			continue
		}
		lines := strings.SplitN(strings.TrimSpace(resp), "\n", 2)
		if len(lines) == 0 || !strings.HasPrefix(strings.ToUpper(strings.TrimSpace(lines[0])), "YES") {
			continue
		}
		rest := ""
		if len(lines) > 1 {
			rest = lines[1]
		}
		if i := strings.Index(strings.ToUpper(rest), "QUOTE:"); i >= 0 {
			q := strings.Trim(strings.TrimSpace(rest[i+len("QUOTE:"):]), "\"'`\u201c\u201d ")
			if q != "" && strings.Contains(message, q) {
				found[obj] = q
			}
		}
	}
	return found
}
