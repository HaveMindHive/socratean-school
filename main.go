package main

import (
	"database/sql"
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"os"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

//go:embed static/*
var staticFiles embed.FS

var db *sql.DB

func main() {
	// Get API key from environment
	apiKey := os.Getenv("ANTHROPIC_API_KEY")

	// Initialize database
	var err error
	db, err = initDB()
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	initSchool()
	detectLocalLLM()
	if cfg, src := resolveLLM(); cfg.Provider == "none" {
		log.Println("No AI model connected. Start Ollama (ollama.com) and pull a model, or set one in the School tab. Until then the tutor asks generic questions.")
	} else {
		log.Printf("AI model: %s %s (tutor) / %s (grading), from %s", cfg.Provider, cfg.Model, cfg.JudgeModel, src)
	}

	// Create API handler
	api := NewAPIHandler(apiKey)

	// Routes
	mux := http.NewServeMux()

	// API endpoints
	mux.HandleFunc("/api/session/create", api.CreateSession)
	mux.HandleFunc("/api/session/get", api.GetSession)
	mux.HandleFunc("/api/profile/analyze", api.AnalyzeProfile)
	mux.HandleFunc("/api/chat", api.Chat)
	mux.HandleFunc("/api/assess", api.Assess)
	mux.HandleFunc("/api/certificate", api.GenerateCertificate)
	mux.HandleFunc("/api/professor/sessions", api.GetAllSessions)
	mux.HandleFunc("/api/professor/session", api.GetSessionDetails)

	// School edition
	mux.HandleFunc("/api/auth/login", api.Login)
	mux.HandleFunc("/api/course/join", api.CourseJoin)
	mux.HandleFunc("/api/admin/school", api.AdminSchool)
	mux.HandleFunc("/api/admin/teachers", api.AdminTeachers)
	mux.HandleFunc("/api/admin/teacher/save", api.AdminTeacherSave)
	mux.HandleFunc("/api/teacher/courses", api.TeacherCourses)
	mux.HandleFunc("/api/teacher/course", api.TeacherCourseGet)
	mux.HandleFunc("/api/teacher/course/create", api.TeacherCourseCreate)
	mux.HandleFunc("/api/teacher/course/update", api.TeacherCourseUpdate)
	mux.HandleFunc("/api/teacher/session/accommodation", api.SessionAccommodation)
	mux.HandleFunc("/api/ai/status", api.AIStatus)
	mux.HandleFunc("/api/admin/llm", api.AdminLLM)
	mux.HandleFunc("/api/admin/llm/models", api.AdminLLMModels)
	mux.HandleFunc("/api/admin/llm/test", api.AdminLLMTest)
	mux.HandleFunc("/api/admin/llm/selftest", api.AdminLLMSelfTest)

	// Static files
	staticFS, _ := fs.Sub(staticFiles, "static")
	mux.Handle("/", http.FileServer(http.FS(staticFS)))

	port := os.Getenv("PORT")
	if port == "" {
		port = "8080"
	}

	fmt.Printf("\n")
	fmt.Printf("╔══════════════════════════════════════════════════════════════╗\n")
	fmt.Printf("║     SOCRATEAN ASSISTANT EDUCATION SYSTEM                     ║\n")
	fmt.Printf("║     The Witness Protocol - Student Mode                      ║\n")
	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║     Server running at: http://localhost:%s                 ║\n", port)
	fmt.Printf("║     Teacher panel: http://localhost:%s/teacher.html      ║\n", port)
	fmt.Printf("╠══════════════════════════════════════════════════════════════╣\n")
	fmt.Printf("║     Concept by T0Rs (The Epoch Project)                      ║\n")
	fmt.Printf("║     [1 = -1]                                                 ║\n")
	fmt.Printf("╚══════════════════════════════════════════════════════════════╝\n")
	fmt.Printf("\n")

	log.Fatal(http.ListenAndServe(":"+port, mux))
}

func initDB() (*sql.DB, error) {
	path := os.Getenv("SOCRATEAN_DB")
	if path == "" {
		path = "./socratean.db"
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	// Create tables
	schema := `
	CREATE TABLE IF NOT EXISTS sessions (
		id TEXT PRIMARY KEY,
		student_name TEXT,
		subject TEXT,
		topic TEXT,
		status TEXT DEFAULT 'profile',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		proficiency INTEGER DEFAULT 0,
		authenticity_score INTEGER DEFAULT 100,
		time_to_proficiency INTEGER DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS profiles (
		session_id TEXT PRIMARY KEY,
		responses TEXT,
		fingerprint TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS exchanges (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		human_input TEXT,
		ai_response TEXT,
		ai_score REAL DEFAULT 0,
		style_match REAL DEFAULT 1,
		concepts_addressed TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS quotes (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		exchange_num INTEGER,
		text TEXT,
		used INTEGER DEFAULT 0,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS concepts (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		name TEXT,
		demonstrated INTEGER DEFAULT 0,
		exchange_num INTEGER,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS warnings (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		type TEXT,
		exchange_num INTEGER,
		score REAL,
		excerpt TEXT,
		timestamp DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);

	CREATE TABLE IF NOT EXISTS papers (
		session_id TEXT PRIMARY KEY,
		title TEXT,
		content TEXT,
		final_score INTEGER,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		FOREIGN KEY (session_id) REFERENCES sessions(id)
	);
	`

	if _, err = db.Exec(schema); err != nil {
		return db, err
	}
	if _, err = db.Exec(schoolSchema); err != nil {
		return db, err
	}
	if _, err = db.Exec(captureSchema); err != nil {
		return db, err
	}
	// additive migrations for databases created by v1; "duplicate column" is expected on reruns
	for _, m := range []string{
		`ALTER TABLE sessions ADD COLUMN course_id INTEGER DEFAULT 0`,
		`ALTER TABLE concepts ADD COLUMN evidence_quote TEXT DEFAULT ''`,
		`ALTER TABLE concepts ADD COLUMN judge TEXT DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN accommodation INTEGER DEFAULT 0`,
		`ALTER TABLE school ADD COLUMN llm TEXT DEFAULT '{}'`,
		`ALTER TABLE school ADD COLUMN llm_key TEXT DEFAULT ''`,
		`ALTER TABLE sessions ADD COLUMN student_key TEXT DEFAULT ''`,
	} {
		db.Exec(m)
	}
	return db, nil
}

const schoolSchema = statusSchema + `
	CREATE TABLE IF NOT EXISTS school (
		id INTEGER PRIMARY KEY CHECK (id = 1),
		name TEXT DEFAULT 'My School',
		admin_salt TEXT,
		admin_hash TEXT,
		policy TEXT DEFAULT '{}'
	);
	CREATE TABLE IF NOT EXISTS teachers (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		name TEXT,
		salt TEXT,
		hash TEXT,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS courses (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		teacher_id INTEGER DEFAULT 0,
		title TEXT,
		syllabus TEXT DEFAULT '',
		join_code TEXT UNIQUE,
		settings TEXT DEFAULT '{}',
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);
	CREATE TABLE IF NOT EXISTS objectives (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		course_id INTEGER,
		position INTEGER,
		text TEXT
	);
	CREATE TABLE IF NOT EXISTS auth_tokens (
		token TEXT PRIMARY KEY,
		role TEXT,
		teacher_id INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

// ==================== API HANDLER ====================

type APIHandler struct {
	apiKey string
	claude *ClaudeClient
}

func NewAPIHandler(apiKey string) *APIHandler {
	return &APIHandler{
		apiKey: apiKey,
		claude: NewClaudeClient(apiKey),
	}
}

// ==================== SESSION MANAGEMENT ====================

type Session struct {
	ID                string    `json:"id"`
	StudentName       string    `json:"student_name"`
	Subject           string    `json:"subject"`
	Topic             string    `json:"topic"`
	Status            string    `json:"status"`
	CreatedAt         time.Time `json:"created_at"`
	UpdatedAt         time.Time `json:"updated_at"`
	Proficiency       int       `json:"proficiency"`
	AuthenticityScore int       `json:"authenticity_score"`
	TimeToProficiency int       `json:"time_to_proficiency"`
	CourseID          int       `json:"course_id"`
	CourseTitle       string    `json:"course_title,omitempty"`
	ReviewSignals     int       `json:"review_signals"`
	HumanLabel        string    `json:"human_label"`
	Accommodation     int       `json:"accommodation"`
}

func (h *APIHandler) CreateSession(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req struct {
		StudentName string `json:"student_name"`
	}
	json.NewDecoder(r.Body).Decode(&req)

	// Insert with empty strings for subject/topic so later Scan into Go string
	// types doesn't fail on NULL (which surfaces as a misleading "Session not found").
	id := uuid.New().String()
	_, err := db.Exec(`
		INSERT INTO sessions (id, student_name, subject, topic, status)
		VALUES (?, ?, '', '', 'profile')`,
		id, req.StudentName)

	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}

	json.NewEncoder(w).Encode(map[string]string{"session_id": id})
}

func (h *APIHandler) GetSession(w http.ResponseWriter, r *http.Request) {
	sessionID := r.URL.Query().Get("id")
	if sessionID == "" {
		http.Error(w, "Session ID required", http.StatusBadRequest)
		return
	}

	var session Session
	err := db.QueryRow(`
		SELECT id, student_name, subject, topic, status, created_at, updated_at,
		       proficiency, authenticity_score, time_to_proficiency
		FROM sessions WHERE id = ?`, sessionID).Scan(
		&session.ID, &session.StudentName, &session.Subject, &session.Topic,
		&session.Status, &session.CreatedAt, &session.UpdatedAt,
		&session.Proficiency, &session.AuthenticityScore, &session.TimeToProficiency)

	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	// Get exchanges
	rows, _ := db.Query(`SELECT human_input, ai_response, ai_score, style_match, timestamp
		FROM exchanges WHERE session_id = ? ORDER BY timestamp`, sessionID)
	defer rows.Close()

	var exchanges []map[string]interface{}
	for rows.Next() {
		var human, ai string
		var aiScore, styleMatch float64
		var ts time.Time
		rows.Scan(&human, &ai, &aiScore, &styleMatch, &ts)
		exchanges = append(exchanges, map[string]interface{}{
			"human":       human,
			"ai":          ai,
			"ai_score":    aiScore,
			"style_match": styleMatch,
			"timestamp":   ts,
		})
	}

	// Get concepts
	conceptRows, _ := db.Query(`SELECT name, demonstrated FROM concepts WHERE session_id = ?`, sessionID)
	defer conceptRows.Close()

	var concepts []map[string]interface{}
	for conceptRows.Next() {
		var name string
		var demonstrated int
		conceptRows.Scan(&name, &demonstrated)
		concepts = append(concepts, map[string]interface{}{
			"name":         name,
			"demonstrated": demonstrated == 1,
		})
	}

	// Get quotes
	quoteRows, _ := db.Query(`SELECT exchange_num, text, used FROM quotes WHERE session_id = ?`, sessionID)
	defer quoteRows.Close()

	var quotes []map[string]interface{}
	for quoteRows.Next() {
		var exchangeNum int
		var text string
		var used int
		quoteRows.Scan(&exchangeNum, &text, &used)
		quotes = append(quotes, map[string]interface{}{
			"exchange": exchangeNum,
			"text":     text,
			"used":     used == 1,
		})
	}

	settings, _, _ := sessionSettings(sessionID)
	json.NewEncoder(w).Encode(map[string]interface{}{
		"session":   session,
		"exchanges": exchanges,
		"concepts":  concepts,
		"quotes":    quotes,
		"settings":  settings,
	})
}

// ==================== PROFILE ANALYSIS ====================

type ProfileAnalysisRequest struct {
	SessionID string    `json:"session_id"`
	Responses []string  `json:"responses"`
	Captures  []Capture `json:"captures"`
}

type WritingFingerprint struct {
	AvgSentenceLength   float64  `json:"avg_sentence_length"`
	AvgWordLength       float64  `json:"avg_word_length"`
	VocabularyRichness  float64  `json:"vocabulary_richness"`
	ContractionUsage    float64  `json:"contraction_usage"`
	PersonalPronounFreq float64  `json:"personal_pronoun_freq"`
	QuestionFreq        float64  `json:"question_freq"`
	CommaFreq           float64  `json:"comma_freq"`
	UniquePatterns      []string `json:"unique_patterns"`
}

func (h *APIHandler) AnalyzeProfile(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ProfileAnalysisRequest
	json.NewDecoder(r.Body).Decode(&req)

	allText := ""
	for _, resp := range req.Responses {
		allText += resp + " "
	}

	// Analyze writing style
	fingerprint := analyzeWritingStyle(allText)

	// Check for AI patterns
	aiScore := detectAIPatterns(allText)
	if aiScore > 0.6 {
		json.NewEncoder(w).Encode(map[string]interface{}{
			"success":  false,
			"error":    "These sound a little formal. Try again the way you would tell a friend. Short and casual is great.",
			"ai_score": aiScore,
		})
		return
	}

	// Use Claude to analyze the profile if API key is available
	var aiAnalysis string
	if h.aiOn() {
		prompt := fmt.Sprintf(`Analyze this student's writing style from their profile responses.
Identify:
1. Their natural voice and tone
2. Common patterns in how they express ideas
3. Vocabulary preferences
4. Any unique stylistic markers

Responses:
%s

Provide a brief analysis (2-3 sentences) that will help identify if future writing matches this student's authentic voice.`, allText)

		aiAnalysis, _ = h.ai("judge", "", prompt)
	}

	// How the voice sample was written (a pasted sample would defeat the fingerprint)
	for i := range req.Captures {
		if i < 4 {
			recordCapture(req.SessionID, "profile", i+1, &req.Captures[i], "")
		}
	}

	// Save to database
	responsesJSON, _ := json.Marshal(req.Responses)
	fingerprintJSON, _ := json.Marshal(fingerprint)

	db.Exec(`INSERT OR REPLACE INTO profiles (session_id, responses, fingerprint) VALUES (?, ?, ?)`,
		req.SessionID, string(responsesJSON), string(fingerprintJSON))

	db.Exec(`UPDATE sessions SET status = 'learning', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		req.SessionID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":     true,
		"fingerprint": fingerprint,
		"ai_analysis": aiAnalysis,
		"ai_score":    aiScore,
	})
}

// ==================== CHAT / SOCRATIC TEACHING ====================

type ChatRequest struct {
	SessionID string   `json:"session_id"`
	Message   string   `json:"message"`
	Subject   string   `json:"subject,omitempty"`
	Topic     string   `json:"topic,omitempty"`
	Capture   *Capture `json:"capture,omitempty"`
}

func (h *APIHandler) Chat(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req ChatRequest
	json.NewDecoder(r.Body).Decode(&req)

	// Get session
	var session Session
	var status string
	err := db.QueryRow(`SELECT status, subject, topic FROM sessions WHERE id = ?`, req.SessionID).
		Scan(&status, &session.Subject, &session.Topic)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}

	settings, guidance, courseID := sessionSettings(req.SessionID)
	style := learningStyle(req.SessionID)
	if style.Hint != "" {
		guidance += "\n" + style.Hint
	}

	// If subject/topic provided, update session. Course sessions already have
	// their subject and objectives from the teacher's course, so they skip this.
	if req.Subject != "" && courseID == 0 {
		db.Exec(`UPDATE sessions SET subject = ?, topic = ?, status = 'learning', updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
			req.Subject, req.Topic, req.SessionID)
		session.Subject = req.Subject
		session.Topic = req.Topic

		// Generate initial concepts
		concepts := generateConcepts(req.Subject)
		for _, c := range concepts {
			db.Exec(`INSERT INTO concepts (session_id, name) VALUES (?, ?)`, req.SessionID, c)
		}
	}

	// Get writing fingerprint
	var fingerprintJSON string
	db.QueryRow(`SELECT fingerprint FROM profiles WHERE session_id = ?`, req.SessionID).Scan(&fingerprintJSON)
	var fingerprint WritingFingerprint
	json.Unmarshal([]byte(fingerprintJSON), &fingerprint)

	// Analyze student input
	aiScore := detectAIPatterns(req.Message)
	styleMatch := compareToFingerprint(req.Message, fingerprint)

	// Log warnings if needed
	if aiScore > 0.4 {
		db.Exec(`INSERT INTO warnings (session_id, type, exchange_num, score, excerpt)
			VALUES (?, 'ai_detected', (SELECT COUNT(*) FROM exchanges WHERE session_id = ?) + 1, ?, ?)`,
			req.SessionID, req.SessionID, aiScore, truncate(req.Message, 100))
	}
	// weak signal: only a strong mismatch on a long answer is worth showing the teacher
	if styleMatch < 0.35 && len(req.Message) > 150 {
		db.Exec(`INSERT INTO warnings (session_id, type, exchange_num, score, excerpt)
			VALUES (?, 'style_mismatch', (SELECT COUNT(*) FROM exchanges WHERE session_id = ?) + 1, ?, ?)`,
			req.SessionID, req.SessionID, styleMatch, truncate(req.Message, 100))
	}

	// Get conversation history
	rows, _ := db.Query(`SELECT human_input, ai_response FROM exchanges WHERE session_id = ? ORDER BY timestamp`, req.SessionID)
	defer rows.Close()

	var history []map[string]string
	for rows.Next() {
		var human, ai string
		rows.Scan(&human, &ai)
		history = append(history, map[string]string{"human": human, "ai": ai})
	}

	// Get concepts status
	conceptRows, _ := db.Query(`SELECT name, demonstrated FROM concepts WHERE session_id = ?`, req.SessionID)

	var conceptsList []string
	var demonstratedCount, totalConcepts int
	for conceptRows.Next() {
		var name string
		var demonstrated int
		conceptRows.Scan(&name, &demonstrated)
		if demonstrated == 0 {
			conceptsList = append(conceptsList, name)
		} else {
			demonstratedCount++
		}
		totalConcepts++
	}
	conceptRows.Close()

	// How this answer was written, recorded before the exchange is saved so a paste is
	// compared with earlier text only. A mostly pasted answer is not the student's own
	// words, so it earns no objective credit and the tutor asks for their own version.
	pastedMost := false
	humanKinds := []string{}
	if req.Capture != nil && strings.TrimSpace(req.Message) != "" {
		var n int
		db.QueryRow(`SELECT COUNT(*) FROM exchanges WHERE session_id = ?`, req.SessionID).Scan(&n)
		req.Capture.FinalLen = utf8.RuneCountInString(req.Message)
		for _, sg := range recordCapture(req.SessionID, "dialogue", n+1, req.Capture, req.Message) {
			if sg.Kind == "pasted_most" {
				pastedMost = true
			}
			if sg.Level == "human" {
				humanKinds = append(humanKinds, sg.Kind)
			}
		}
	}
	if pastedMost {
		guidance += "\nThe student's last message was pasted in, not written by them. Do not praise it as their understanding. Ask them, kindly and without any suspicion, to explain the idea in their own words, in a sentence or two."
	}

	// Judge which open objectives this message demonstrates, before the tutor replies,
	// so the next question aims at what is still open.
	judged, judgeMethod := map[string]string{}, "pasted"
	if !pastedMost {
		judged, judgeMethod = h.judgeObjectives(session.Subject, conceptsList, history, req.Message)
	}
	var stillOpen []string
	var conceptsAddressed []string
	for _, c := range conceptsList {
		if quote, ok := judged[c]; ok {
			conceptsAddressed = append(conceptsAddressed, c)
			db.Exec(`UPDATE concepts SET demonstrated = 1, evidence_quote = ?, judge = ?,
				exchange_num = (SELECT COUNT(*) FROM exchanges WHERE session_id = ?) WHERE session_id = ? AND name = ?`,
				quote, judgeMethod, req.SessionID, req.SessionID, c)
			demonstratedCount++
		} else {
			stillOpen = append(stillOpen, c)
		}
	}

	// Generate AI response
	var aiResponse string
	if h.aiOn() {
		aiResponse = h.generateSocraticResponse(session.Subject, session.Topic, req.Message, history, stillOpen, guidance)
	} else {
		aiResponse = generateFallbackResponse(stillOpen)
	}

	// Calculate proficiency
	proficiency := 0
	if totalConcepts > 0 {
		proficiency = (demonstratedCount * 100) / totalConcepts
	}

	// Save exchange
	db.Exec(`INSERT INTO exchanges (session_id, human_input, ai_response, ai_score, style_match, concepts_addressed) VALUES (?, ?, ?, ?, ?, ?)`,
		req.SessionID, req.Message, aiResponse, aiScore, styleMatch, strings.Join(conceptsAddressed, ","))

	// Extract quotes
	extractAndSaveQuotes(req.SessionID, req.Message)

	// Update session
	db.Exec(`UPDATE sessions SET proficiency = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		proficiency, req.SessionID)

	// Count exchanges
	var exchangeCount int
	db.QueryRow(`SELECT COUNT(*) FROM exchanges WHERE session_id = ?`, req.SessionID).Scan(&exchangeCount)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"response":            aiResponse,
		"ai_score":            aiScore,
		"style_match":         styleMatch,
		"proficiency":         proficiency,
		"exchange_count":      exchangeCount,
		"concepts_addressed":  conceptsAddressed,
		"proficiency_reached": proficiency >= settings.GraspThreshold,
		"grasp_threshold":     settings.GraspThreshold,
		"judge":               judgeMethod,
		"human_signs":         humanKinds,
		"learning_style":      style.Names,
	})
}

func (h *APIHandler) generateSocraticResponse(subject, topic, message string, history []map[string]string, remainingConcepts []string, guidance string) string {
	// Build conversation history for context
	historyText := ""
	for _, h := range history {
		historyText += fmt.Sprintf("Student: %s\nTeacher: %s\n\n", h["human"], h["ai"])
	}

	conceptsText := "Concepts still to address: " + strings.Join(remainingConcepts, ", ")
	if len(remainingConcepts) == 0 {
		conceptsText = "All core concepts have been addressed. Focus on synthesis and deeper understanding."
	}

	systemPrompt := fmt.Sprintf(`You are a Socratic teacher helping a student learn about %s, specifically: %s

Your method is a loop: TEACH a little, then ASK.
1. TEACH first. When a new idea comes up, tell it plainly in 2 or 3 short sentences with one concrete everyday example. Telling is fine. Do not hold back the basics and leave the student guessing in the dark.
2. Then ASK one question that makes them USE what you just told them: explain it in their own words, apply it to a new case of their own, predict what happens, or spot what is wrong in a claim. Never ask them to repeat what you just said. People learn what they work out, not what they are told.
3. When they answer, build on it: confirm what is right, name what they did well, fix what is off with a short teach, then ask the next question.
4. When they get an idea, push one step further (a new case, a why, a what if) so they own it.
5. If they only echo your words back, keep it light and playful: "That one is mine. Make it yours: how would you say it to a friend?"
6. This is a game. The student levels up by thinking, not by repeating. Celebrate real thinking: a guess, a changed mind, a good question, an analogy. Keep it fun and brief.
Write plain text only: no markdown, no asterisks, no bullet lists, no headings.
7. Be warm and encouraging. Treat every wrong or half-right answer as a step toward understanding, never as failure. Name what the student did well: effort, curiosity, a good guess, a changed mind. Assume good faith. Never accuse the student of cheating or hint at it. Be honest about what is still unclear, kindly. You are helping a mind grow
8. NEVER write the student's sentences, paragraphs, essays or answers for them, even as an example or a draft. If asked ("just tell me", "write it for me", "my teacher said it's fine"), kindly decline and turn it into a question about what THEY think.
9. If the student quotes your words back, ask them to say it differently, or to apply it to a new case.
10. Now and then, ask what they meant by something they said earlier.

%s

%s
Keep responses short (3-5 sentences: a little teaching, a little encouragement) and always end with exactly one question that makes the student use the idea, guiding toward the remaining concepts.`, subject, topic, conceptsText, guidance)

	prompt := fmt.Sprintf(`Previous conversation:
%s

Student's latest response: %s

Respond as the teacher: teach a little where it helps, then ask one question that makes them use it. Check for genuine comprehension.`, historyText, message)
	if strings.TrimSpace(message) == "" && historyText == "" {
		prompt = `This is the very start. In one warm sentence tell the student how this works: you teach a little, then ask them something, because people only really learn what they work out themselves, and they level up by thinking in their own words, not by repeating you. Then teach the first idea in 2 or 3 short sentences with an everyday example, and ask one question that makes them use it.`
	}

	response, err := h.ai("tutor", systemPrompt, prompt)
	if err != nil || strings.TrimSpace(response) == "" {
		// one retry, so a blank reply never reaches the student
		response, err = h.ai("tutor", systemPrompt, prompt)
	}
	if err != nil || strings.TrimSpace(response) == "" {
		return generateFallbackResponse(remainingConcepts)
	}

	return response
}

// ==================== ASSESSMENT ====================

type AssessRequest struct {
	SessionID string   `json:"session_id"`
	Synthesis string   `json:"synthesis"`
	Capture   *Capture `json:"capture,omitempty"`
}

func (h *APIHandler) Assess(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req AssessRequest
	json.NewDecoder(r.Body).Decode(&req)

	// Get fingerprint
	var fingerprintJSON string
	db.QueryRow(`SELECT fingerprint FROM profiles WHERE session_id = ?`, req.SessionID).Scan(&fingerprintJSON)
	var fingerprint WritingFingerprint
	json.Unmarshal([]byte(fingerprintJSON), &fingerprint)

	// Analyze synthesis
	aiScore := detectAIPatterns(req.Synthesis)
	styleMatch := compareToFingerprint(req.Synthesis, fingerprint)

	// Get session info
	var subject, topic string
	db.QueryRow(`SELECT subject, topic FROM sessions WHERE id = ?`, req.SessionID).Scan(&subject, &topic)

	// Use Claude to evaluate mastery
	var masteryEvaluation string
	var masteryLevel string = "Developing"

	if h.aiOn() {
		prompt := fmt.Sprintf(`Evaluate this student's synthesis for mastery of %s - %s:

"%s"

Assess:
1. Accuracy of understanding (0-100)
2. Depth of insight (0-100)
3. Original thinking vs regurgitation (0-100)
4. Clarity of expression (0-100)

Provide a brief evaluation (2-3 sentences) and an overall mastery level: "Beginning", "Developing", "Proficient", or "Mastery"

Format your response as:
EVALUATION: [your evaluation]
LEVEL: [mastery level]
SCORES: accuracy=[X], depth=[X], originality=[X], clarity=[X]`, subject, topic, req.Synthesis)

		masteryEvaluation, _ = h.ai("judge", "", prompt)

		// Parse mastery level from response
		if strings.Contains(masteryEvaluation, "Mastery") {
			masteryLevel = "Mastery"
		} else if strings.Contains(masteryEvaluation, "Proficient") {
			masteryLevel = "Proficient"
		} else if strings.Contains(masteryEvaluation, "Developing") {
			masteryLevel = "Developing"
		} else {
			masteryLevel = "Beginning"
		}
	}

	recordCapture(req.SessionID, "synthesis", 0, req.Capture, req.Synthesis)

	// Extract quotes from synthesis
	extractAndSaveQuotes(req.SessionID, req.Synthesis)

	// Calculate time to proficiency
	var createdAt time.Time
	db.QueryRow(`SELECT created_at FROM sessions WHERE id = ?`, req.SessionID).Scan(&createdAt)
	timeToProficiency := int(time.Since(createdAt).Minutes())

	// Update session
	db.Exec(`UPDATE sessions SET status = 'building', time_to_proficiency = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		timeToProficiency, req.SessionID)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"success":             true,
		"ai_score":            aiScore,
		"style_match":         styleMatch,
		"mastery_evaluation":  masteryEvaluation,
		"mastery_level":       masteryLevel,
		"time_to_proficiency": timeToProficiency,
	})
}

// ==================== CERTIFICATE GENERATION ====================

type CertificateRequest struct {
	SessionID string   `json:"session_id"`
	Title     string   `json:"title"`
	Content   string   `json:"content"`
	Capture   *Capture `json:"capture,omitempty"`
}

func (h *APIHandler) GenerateCertificate(w http.ResponseWriter, r *http.Request) {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	var req CertificateRequest
	json.NewDecoder(r.Body).Decode(&req)

	// Get session data
	var session Session
	db.QueryRow(`SELECT id, student_name, subject, topic, proficiency, time_to_proficiency
		FROM sessions WHERE id = ?`, req.SessionID).Scan(
		&session.ID, &session.StudentName, &session.Subject, &session.Topic,
		&session.Proficiency, &session.TimeToProficiency)

	// Get fingerprint
	var fingerprintJSON string
	db.QueryRow(`SELECT fingerprint FROM profiles WHERE session_id = ?`, req.SessionID).Scan(&fingerprintJSON)
	var fingerprint WritingFingerprint
	json.Unmarshal([]byte(fingerprintJSON), &fingerprint)

	// Analyze final paper
	aiScore := detectAIPatterns(req.Content)
	styleMatch := compareToFingerprint(req.Content, fingerprint)

	// Get warnings
	warningRows, _ := db.Query(`SELECT type, exchange_num, score FROM warnings WHERE session_id = ?`, req.SessionID)
	defer warningRows.Close()

	var warnings []map[string]interface{}
	for warningRows.Next() {
		var wType string
		var exchangeNum int
		var score float64
		warningRows.Scan(&wType, &exchangeNum, &score)
		warnings = append(warnings, map[string]interface{}{
			"type":     wType,
			"exchange": exchangeNum,
			"score":    score,
		})
	}

	// Calculate final authenticity score: capture facts first, weak text signals capped
	recordCapture(req.SessionID, "paper", 0, req.Capture, req.Content)
	authenticityScore := computeAuthenticity(req.SessionID, aiScore, styleMatch)
	typedPct, haveTyped := typedShare(req.SessionID)

	// Get exchange count
	var exchangeCount int
	db.QueryRow(`SELECT COUNT(*) FROM exchanges WHERE session_id = ?`, req.SessionID).Scan(&exchangeCount)

	// Save paper
	db.Exec(`INSERT OR REPLACE INTO papers (session_id, title, content, final_score) VALUES (?, ?, ?, ?)`,
		req.SessionID, req.Title, req.Content, authenticityScore)

	var priorStatus, studentKey string
	db.QueryRow(`SELECT status, COALESCE(student_key, '') FROM sessions WHERE id = ?`, req.SessionID).Scan(&priorStatus, &studentKey)
	human := humanEvidence(req.SessionID)
	var scholar map[string]interface{}
	if priorStatus != "certified" { // a work counts once
		humanLabel, _ := human["label"].(string)
		scholar = updateStatus(studentKey, isWitnessed(authenticityScore, typedPct, haveTyped, humanLabel))
	}

	// Update session
	db.Exec(`UPDATE sessions SET status = 'certified', authenticity_score = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		authenticityScore, req.SessionID)

	// Generate certificate
	certificate := map[string]interface{}{
		"session_id":          session.ID,
		"student_name":        session.StudentName,
		"subject":             session.Subject,
		"topic":               session.Topic,
		"paper_title":         req.Title,
		"authenticity_score":  authenticityScore,
		"time_to_proficiency": session.TimeToProficiency,
		"exchange_count":      exchangeCount,
		"proficiency":         session.Proficiency,
		"style_match":         int(styleMatch * 100),
		"human_evidence":      human,
		"scholar":             scholar,
		"learning_style":      learningStyle(req.SessionID).Names,
		"ai_detection":        aiScore < 0.3,
		"warnings":            warnings,
		"generated_at":        time.Now(),
		"mastery_level":       getMasteryLevel(session.Proficiency),
		"typed_share":         typedPct,
		"typed_share_known":   haveTyped,
	}

	json.NewEncoder(w).Encode(certificate)
}

// ==================== PROFESSOR DASHBOARD ====================

func (h *APIHandler) GetAllSessions(w http.ResponseWriter, r *http.Request) {
	id, ok := requireAuth(w, r, "teacher", "admin")
	if !ok {
		return
	}
	// a teacher sees only students in their own courses; the school admin sees everyone
	q := `SELECT s.id, s.student_name, s.subject, s.topic, s.status, s.created_at,
		       s.proficiency, s.authenticity_score, s.time_to_proficiency,
		       COALESCE(s.course_id, 0), COALESCE(c.title, ''),
		       (SELECT COALESCE(SUM(review_count), 0) FROM capture_events e WHERE e.session_id = s.id)
		FROM sessions s LEFT JOIN courses c ON c.id = s.course_id`
	args := []interface{}{}
	if id.Role == "teacher" {
		q += ` WHERE c.teacher_id = ?`
		args = append(args, id.TeacherID)
	}
	q += ` ORDER BY s.created_at DESC`
	rows, err := db.Query(q, args...)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	defer rows.Close()

	sessions := []Session{}
	for rows.Next() {
		var s Session
		rows.Scan(&s.ID, &s.StudentName, &s.Subject, &s.Topic, &s.Status,
			&s.CreatedAt, &s.Proficiency, &s.AuthenticityScore, &s.TimeToProficiency,
			&s.CourseID, &s.CourseTitle, &s.ReviewSignals)
		sessions = append(sessions, s)
	}
	rows.Close()
	for i := range sessions {
		sessions[i].HumanLabel, _ = humanEvidence(sessions[i].ID)["label"].(string)
	}

	writeJSON(w, sessions)
}

func (h *APIHandler) GetSessionDetails(w http.ResponseWriter, r *http.Request) {
	id, ok := requireAuth(w, r, "teacher", "admin")
	if !ok {
		return
	}
	sessionID := r.URL.Query().Get("id")

	// Get full session data including all exchanges, quotes, warnings
	var session Session
	err := db.QueryRow(`SELECT id, student_name, subject, topic, status, created_at, updated_at,
		proficiency, authenticity_score, time_to_proficiency, COALESCE(course_id, 0), COALESCE(accommodation, 0)
		FROM sessions WHERE id = ?`, sessionID).Scan(
		&session.ID, &session.StudentName, &session.Subject, &session.Topic,
		&session.Status, &session.CreatedAt, &session.UpdatedAt,
		&session.Proficiency, &session.AuthenticityScore, &session.TimeToProficiency, &session.CourseID, &session.Accommodation)
	if err != nil {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	if id.Role == "teacher" {
		var owner int
		db.QueryRow(`SELECT teacher_id FROM courses WHERE id = ?`, session.CourseID).Scan(&owner)
		if session.CourseID == 0 || owner != id.TeacherID {
			http.Error(w, "Session not found", http.StatusNotFound)
			return
		}
	}

	// How each answer was written
	capRows, _ := db.Query(`SELECT context, exchange_num, summary, signals FROM capture_events WHERE session_id = ? ORDER BY id`, sessionID)
	captures := []map[string]interface{}{}
	for capRows.Next() {
		var ctx, sum, sj string
		var n int
		capRows.Scan(&ctx, &n, &sum, &sj)
		var c Capture
		var sigs []Signal
		json.Unmarshal([]byte(sum), &c)
		json.Unmarshal([]byte(sj), &sigs)
		if sigs == nil {
			sigs = []Signal{}
		}
		captures = append(captures, map[string]interface{}{"context": ctx, "exchange": n, "capture": c, "signals": sigs})
	}
	capRows.Close()

	// Objectives with the student's own words as evidence
	objRows, _ := db.Query(`SELECT name, demonstrated, COALESCE(evidence_quote, ''), COALESCE(judge, '') FROM concepts WHERE session_id = ?`, sessionID)
	objectives := []map[string]interface{}{}
	for objRows.Next() {
		var name, quote, judge string
		var demonstrated int
		objRows.Scan(&name, &demonstrated, &quote, &judge)
		objectives = append(objectives, map[string]interface{}{
			"name": name, "demonstrated": demonstrated == 1, "evidence": quote, "judge": judge,
		})
	}
	objRows.Close()

	// Get all exchanges with full detail
	exchangeRows, _ := db.Query(`
		SELECT human_input, ai_response, ai_score, style_match, concepts_addressed, timestamp
		FROM exchanges WHERE session_id = ? ORDER BY timestamp`, sessionID)
	defer exchangeRows.Close()

	var exchanges []map[string]interface{}
	for exchangeRows.Next() {
		var human, ai, concepts string
		var aiScore, styleMatch float64
		var ts time.Time
		exchangeRows.Scan(&human, &ai, &aiScore, &styleMatch, &concepts, &ts)
		exchanges = append(exchanges, map[string]interface{}{
			"human":              human,
			"ai":                 ai,
			"ai_score":           aiScore,
			"style_match":        styleMatch,
			"concepts_addressed": concepts,
			"timestamp":          ts,
		})
	}

	// Get warnings
	warningRows, _ := db.Query(`SELECT type, exchange_num, score, excerpt, timestamp FROM warnings WHERE session_id = ?`, sessionID)
	defer warningRows.Close()

	var warnings []map[string]interface{}
	for warningRows.Next() {
		var wType, excerpt string
		var exchangeNum int
		var score float64
		var ts time.Time
		warningRows.Scan(&wType, &exchangeNum, &score, &excerpt, &ts)
		warnings = append(warnings, map[string]interface{}{
			"type":      wType,
			"exchange":  exchangeNum,
			"score":     score,
			"excerpt":   excerpt,
			"timestamp": ts,
		})
	}

	// Get paper if exists
	var paperTitle, paperContent string
	var paperScore int
	db.QueryRow(`SELECT title, content, final_score FROM papers WHERE session_id = ?`, sessionID).
		Scan(&paperTitle, &paperContent, &paperScore)

	json.NewEncoder(w).Encode(map[string]interface{}{
		"session":    session,
		"objectives": objectives,
		"captures":   captures,
		"human":      humanEvidence(sessionID),
		"style":      learningStyle(sessionID),
		"exchanges":  exchanges,
		"warnings":   warnings,
		"paper": map[string]interface{}{
			"title":   paperTitle,
			"content": paperContent,
			"score":   paperScore,
		},
	})
}

// ==================== HELPER FUNCTIONS ====================

func generateConcepts(subject string) []string {
	conceptMaps := map[string][]string{
		"philosophy":  {"Core definition", "Historical context", "Key arguments", "Counter-arguments", "Personal interpretation"},
		"physics":     {"Fundamental principles", "Mathematical basis", "Real-world applications", "Experimental evidence", "Implications"},
		"history":     {"Timeline & context", "Key figures", "Causes & effects", "Primary sources", "Modern relevance"},
		"biology":     {"Basic mechanisms", "Cellular level", "Organism level", "Ecological context", "Applications"},
		"mathematics": {"Core theorem", "Proof understanding", "Applications", "Related concepts", "Problem-solving"},
		"literature":  {"Plot & structure", "Themes", "Character analysis", "Historical context", "Personal interpretation"},
		"psychology":  {"Core theory", "Research basis", "Practical applications", "Critiques", "Modern developments"},
	}

	if concepts, ok := conceptMaps[strings.ToLower(subject)]; ok {
		return concepts
	}
	return conceptMaps["philosophy"]
}

func generateFallbackResponse(remainingConcepts []string) string {
	if len(remainingConcepts) == 0 {
		return "Excellent work! You've demonstrated understanding of all the core concepts. Please proceed to the synthesis phase to show your comprehensive understanding."
	}

	responses := []string{
		fmt.Sprintf("Interesting perspective. Can you elaborate on how this connects to %s?", remainingConcepts[0]),
		fmt.Sprintf("I see your point. What about %s - how does that factor into your understanding?", remainingConcepts[0]),
		fmt.Sprintf("Good thinking. Now consider %s - what's your take on that aspect?", remainingConcepts[0]),
	}

	return responses[time.Now().UnixNano()%int64(len(responses))]
}

func evaluateConceptsAddressed(message string, concepts []string) []string {
	var addressed []string
	messageLower := strings.ToLower(message)

	for _, concept := range concepts {
		keywords := strings.Split(strings.ToLower(concept), " ")
		matchCount := 0
		for _, kw := range keywords {
			if len(kw) > 3 && strings.Contains(messageLower, kw) {
				matchCount++
			}
		}
		// If message is substantial and touches on concept keywords
		if (matchCount > 0 && len(message) > 100) || matchCount >= 2 {
			addressed = append(addressed, concept)
		}
	}

	return addressed
}

func extractAndSaveQuotes(sessionID, text string) {
	sentences := strings.FieldsFunc(text, func(r rune) bool {
		return r == '.' || r == '!' || r == '?'
	})

	var exchangeNum int
	db.QueryRow(`SELECT COUNT(*) FROM exchanges WHERE session_id = ?`, sessionID).Scan(&exchangeNum)

	for _, s := range sentences {
		s = strings.TrimSpace(s)
		if len(s) > 30 {
			db.Exec(`INSERT INTO quotes (session_id, exchange_num, text) VALUES (?, ?, ?)`,
				sessionID, exchangeNum, s)
		}
	}
}

func getMasteryLevel(proficiency int) string {
	if proficiency >= 90 {
		return "Mastery"
	} else if proficiency >= 80 {
		return "Proficient"
	} else if proficiency >= 60 {
		return "Developing"
	}
	return "Beginning"
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen-3] + "..."
}
