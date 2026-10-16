package main

// School edition: the school is the macro layer (defaults, locks, house guidance),
// each teacher is a micro layer (courses with their own syllabus, objectives, settings).
// A course setting overrides the school default unless the school has locked that key.

import (
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"log"
	"math/big"
	"net/http"
	"os"
	"regexp"
	"strings"
	"time"
)

// ==================== SETTINGS: SCHOOL DEFAULTS + COURSE OVERRIDES ====================

type Settings struct {
	GraspThreshold int      `json:"grasp_threshold"`
	PastePolicy    string   `json:"paste_policy"` // "own" (default: paste only their own saved words), "block" or "record"
	Outputs        []string `json:"outputs"`      // "paper", "test"
	ReadingLevel   string   `json:"reading_level"`
	TutorGuidance  string   `json:"tutor_guidance"`
}

type SchoolPolicy struct {
	Defaults Settings `json:"defaults"`
	Locked   []string `json:"locked"`   // setting keys teachers cannot change
	Guidance string   `json:"guidance"` // school-wide tutor guidance, always applies
}

var settingKeys = []string{"grasp_threshold", "paste_policy", "outputs", "reading_level", "tutor_guidance"}

func normalizeSettings(s Settings) Settings {
	if s.GraspThreshold < 50 || s.GraspThreshold > 100 {
		s.GraspThreshold = 80
	}
	if s.PastePolicy != "record" && s.PastePolicy != "block" && s.PastePolicy != "own" {
		s.PastePolicy = "own"
	}
	var outs []string
	for _, o := range s.Outputs {
		if o == "paper" || o == "test" {
			outs = append(outs, o)
		}
	}
	if len(outs) == 0 {
		outs = []string{"paper", "test"}
	}
	s.Outputs = outs
	return s
}

func normalizePolicy(p SchoolPolicy) SchoolPolicy {
	p.Defaults = normalizeSettings(p.Defaults)
	var locked []string
	for _, k := range p.Locked {
		for _, ok := range settingKeys {
			if k == ok {
				locked = append(locked, k)
			}
		}
	}
	p.Locked = locked
	return p
}

// effectiveSettings applies a course's overrides on top of the school defaults, skipping locked keys.
func effectiveSettings(p SchoolPolicy, courseJSON string) Settings {
	p = normalizePolicy(p)
	s := p.Defaults
	var over map[string]json.RawMessage
	json.Unmarshal([]byte(courseJSON), &over)
	locked := map[string]bool{}
	for _, k := range p.Locked {
		locked[k] = true
	}
	set := func(k string, dst interface{}) {
		if raw, ok := over[k]; ok && !locked[k] {
			json.Unmarshal(raw, dst)
		}
	}
	set("grasp_threshold", &s.GraspThreshold)
	set("paste_policy", &s.PastePolicy)
	set("outputs", &s.Outputs)
	set("reading_level", &s.ReadingLevel)
	set("tutor_guidance", &s.TutorGuidance)
	return normalizeSettings(s)
}

// tutorGuidance is what gets appended to the tutor prompt: school house rules first, then the course's own.
func tutorGuidance(p SchoolPolicy, s Settings) string {
	var parts []string
	if g := strings.TrimSpace(p.Guidance); g != "" {
		parts = append(parts, "School guidance: "+g)
	}
	if g := strings.TrimSpace(s.TutorGuidance); g != "" {
		parts = append(parts, "Course guidance: "+g)
	}
	if l := strings.TrimSpace(s.ReadingLevel); l != "" {
		parts = append(parts, "Pitch your questions and vocabulary for this level: "+l+".")
	}
	return strings.Join(parts, "\n")
}

func loadPolicy() SchoolPolicy {
	var raw string
	db.QueryRow(`SELECT policy FROM school WHERE id = 1`).Scan(&raw)
	var p SchoolPolicy
	json.Unmarshal([]byte(raw), &p)
	return normalizePolicy(p)
}

// ==================== PASSCODES AND SESSIONS ====================

const codeAlphabet = "ABCDEFGHJKLMNPQRSTUVWXYZ23456789" // no 0/O/1/I

func randomCode(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(codeAlphabet)))
	for i := range b {
		v, _ := rand.Int(rand.Reader, max)
		b[i] = codeAlphabet[v.Int64()]
	}
	return string(b)
}

func randomHex(n int) string {
	b := make([]byte, n)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func hashPass(salt, pass string) string {
	h := sha256.Sum256([]byte(salt + pass))
	return hex.EncodeToString(h[:])
}

func passMatches(salt, hash, pass string) bool {
	return subtle.ConstantTimeCompare([]byte(hashPass(salt, pass)), []byte(hash)) == 1
}

// initSchool creates the school row and the admin passcode on first run.
func initSchool() {
	db.Exec(`INSERT OR IGNORE INTO school (id, name, policy) VALUES (1, 'My School', '{}')`)
	var hash string
	db.QueryRow(`SELECT COALESCE(admin_hash, '') FROM school WHERE id = 1`).Scan(&hash)
	if hash != "" {
		return
	}
	pass := os.Getenv("SOCRATEAN_ADMIN_PASSCODE")
	generated := pass == ""
	if generated {
		pass = randomCode(12)
	}
	salt := randomHex(8)
	db.Exec(`UPDATE school SET admin_salt = ?, admin_hash = ? WHERE id = 1`, salt, hashPass(salt, pass))
	if generated {
		fmt.Printf("\n  FIRST RUN: school admin passcode is  %s\n  Write it down. It is shown once. Set SOCRATEAN_ADMIN_PASSCODE to choose your own.\n\n", pass)
	}
}

type identity struct {
	Role      string // "admin" or "teacher"
	TeacherID int
}

func newToken(role string, teacherID int) string {
	t := randomHex(24)
	db.Exec(`INSERT INTO auth_tokens (token, role, teacher_id) VALUES (?, ?, ?)`, t, role, teacherID)
	return t
}

// requireAuth returns the caller's identity or writes a 401/403 and returns false.
func requireAuth(w http.ResponseWriter, r *http.Request, roles ...string) (identity, bool) {
	tok := strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
	var id identity
	err := db.QueryRow(`SELECT role, teacher_id FROM auth_tokens
		WHERE token = ? AND created_at > datetime('now', '-12 hours')`, tok).Scan(&id.Role, &id.TeacherID)
	if tok == "" || err != nil {
		http.Error(w, "Sign in required", http.StatusUnauthorized)
		return id, false
	}
	for _, role := range roles {
		if id.Role == role {
			return id, true
		}
	}
	http.Error(w, "Not allowed", http.StatusForbidden)
	return id, false
}

func writeJSON(w http.ResponseWriter, v interface{}) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(v)
}

func readJSON(w http.ResponseWriter, r *http.Request, v interface{}) bool {
	if r.Method != "POST" {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return false
	}
	r.Body = http.MaxBytesReader(w, r.Body, 2<<20)
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		http.Error(w, "Bad request", http.StatusBadRequest)
		return false
	}
	return true
}

func schoolName() string {
	var n string
	db.QueryRow(`SELECT name FROM school WHERE id = 1`).Scan(&n)
	return n
}

func (h *APIHandler) Login(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Passcode string `json:"passcode"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	pass := strings.ToUpper(strings.TrimSpace(req.Passcode))
	// admin passcodes chosen via env may be mixed case, so try the raw value too
	raw := strings.TrimSpace(req.Passcode)

	var salt, hash string
	db.QueryRow(`SELECT COALESCE(admin_salt,''), COALESCE(admin_hash,'') FROM school WHERE id = 1`).Scan(&salt, &hash)
	if pass != "" && (passMatches(salt, hash, pass) || passMatches(salt, hash, raw)) {
		writeJSON(w, map[string]interface{}{"token": newToken("admin", 0), "role": "admin", "name": "School admin", "school": schoolName()})
		return
	}
	rows, _ := db.Query(`SELECT id, name, salt, hash FROM teachers`)
	matchID, matchName := 0, ""
	for rows.Next() {
		var id int
		var name, tsalt, thash string
		rows.Scan(&id, &name, &tsalt, &thash)
		if pass != "" && passMatches(tsalt, thash, pass) {
			matchID, matchName = id, name
		}
	}
	rows.Close() // must close before writing the token: SQLite locks the table while a cursor is open
	if matchID != 0 {
		writeJSON(w, map[string]interface{}{"token": newToken("teacher", matchID), "role": "teacher", "name": matchName, "teacher_id": matchID, "school": schoolName()})
		return
	}
	time.Sleep(600 * time.Millisecond) // slow down guessing
	http.Error(w, "That passcode did not match.", http.StatusUnauthorized)
}

// ==================== ADMIN: SCHOOL POLICY AND TEACHERS ====================

func (h *APIHandler) AdminSchool(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "admin"); !ok {
		return
	}
	if r.Method == "POST" {
		var req struct {
			Name   string       `json:"name"`
			Policy SchoolPolicy `json:"policy"`
		}
		if !readJSON(w, r, &req) {
			return
		}
		pol, _ := json.Marshal(normalizePolicy(req.Policy))
		name := strings.TrimSpace(req.Name)
		if name == "" {
			name = "My School"
		}
		db.Exec(`UPDATE school SET name = ?, policy = ? WHERE id = 1`, name, string(pol))
	}
	var name string
	db.QueryRow(`SELECT name FROM school WHERE id = 1`).Scan(&name)
	writeJSON(w, map[string]interface{}{"name": name, "policy": loadPolicy()})
}

func (h *APIHandler) AdminTeachers(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "admin"); !ok {
		return
	}
	rows, _ := db.Query(`SELECT t.id, t.name, (SELECT COUNT(*) FROM courses c WHERE c.teacher_id = t.id)
		FROM teachers t ORDER BY t.name`)
	defer rows.Close()
	out := []map[string]interface{}{}
	for rows.Next() {
		var id, courses int
		var name string
		rows.Scan(&id, &name, &courses)
		out = append(out, map[string]interface{}{"id": id, "name": name, "courses": courses})
	}
	writeJSON(w, out)
}

// AdminTeacherSave creates a teacher (no id) or resets their passcode (id). The passcode is returned once.
func (h *APIHandler) AdminTeacherSave(w http.ResponseWriter, r *http.Request) {
	if _, ok := requireAuth(w, r, "admin"); !ok {
		return
	}
	var req struct {
		ID   int    `json:"id"`
		Name string `json:"name"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	pass := randomCode(10)
	salt := randomHex(8)
	if req.ID == 0 {
		name := strings.TrimSpace(req.Name)
		if name == "" {
			http.Error(w, "Teacher name required", http.StatusBadRequest)
			return
		}
		res, err := db.Exec(`INSERT INTO teachers (name, salt, hash) VALUES (?, ?, ?)`, name, salt, hashPass(salt, pass))
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		id, _ := res.LastInsertId()
		req.ID = int(id)
		req.Name = name
	} else {
		db.Exec(`UPDATE teachers SET salt = ?, hash = ? WHERE id = ?`, salt, hashPass(salt, pass), req.ID)
		db.Exec(`DELETE FROM auth_tokens WHERE role = 'teacher' AND teacher_id = ?`, req.ID)
	}
	writeJSON(w, map[string]interface{}{"id": req.ID, "name": req.Name, "passcode": pass})
}

// ==================== TEACHER: COURSES ====================

type CourseView struct {
	ID         int             `json:"id"`
	TeacherID  int             `json:"teacher_id"`
	Title      string          `json:"title"`
	Syllabus   string          `json:"syllabus"`
	JoinCode   string          `json:"join_code"`
	Overrides  json.RawMessage `json:"overrides"`
	Effective  Settings        `json:"effective"`
	Defaults   Settings        `json:"defaults"`
	Guidance   string          `json:"school_guidance"`
	Locked     []string        `json:"locked"`
	Objectives []string        `json:"objectives"`
	Students   int             `json:"students"`
}

func loadCourse(id int) (CourseView, bool) {
	var c CourseView
	var overrides string
	err := db.QueryRow(`SELECT id, teacher_id, title, syllabus, join_code, settings FROM courses WHERE id = ?`, id).
		Scan(&c.ID, &c.TeacherID, &c.Title, &c.Syllabus, &c.JoinCode, &overrides)
	if err != nil {
		return c, false
	}
	pol := loadPolicy()
	c.Overrides = json.RawMessage(overrides)
	c.Effective = effectiveSettings(pol, overrides)
	c.Defaults = pol.Defaults
	c.Guidance = pol.Guidance
	c.Locked = pol.Locked
	c.Objectives = loadObjectives(id)
	db.QueryRow(`SELECT COUNT(*) FROM sessions WHERE course_id = ?`, id).Scan(&c.Students)
	return c, true
}

func loadObjectives(courseID int) []string {
	rows, _ := db.Query(`SELECT text FROM objectives WHERE course_id = ? ORDER BY position`, courseID)
	defer rows.Close()
	out := []string{}
	for rows.Next() {
		var t string
		rows.Scan(&t)
		out = append(out, t)
	}
	return out
}

func saveObjectives(courseID int, objs []string) {
	db.Exec(`DELETE FROM objectives WHERE course_id = ?`, courseID)
	pos := 0
	for _, o := range objs {
		o = strings.TrimSpace(o)
		if o == "" || len(o) > 300 {
			continue
		}
		db.Exec(`INSERT INTO objectives (course_id, position, text) VALUES (?, ?, ?)`, courseID, pos, o)
		pos++
	}
}

// ownsCourse: admin owns everything, a teacher owns their own courses.
func ownsCourse(id identity, c CourseView) bool {
	return id.Role == "admin" || c.TeacherID == id.TeacherID
}

func cleanOverrides(raw json.RawMessage) string {
	var m map[string]json.RawMessage
	if json.Unmarshal(raw, &m) != nil {
		return "{}"
	}
	keep := map[string]json.RawMessage{}
	for _, k := range settingKeys {
		if v, ok := m[k]; ok {
			keep[k] = v
		}
	}
	b, _ := json.Marshal(keep)
	return string(b)
}

func (h *APIHandler) TeacherCourses(w http.ResponseWriter, r *http.Request) {
	id, ok := requireAuth(w, r, "teacher", "admin")
	if !ok {
		return
	}
	q := `SELECT id FROM courses ORDER BY created_at DESC`
	args := []interface{}{}
	if id.Role == "teacher" {
		q = `SELECT id FROM courses WHERE teacher_id = ? ORDER BY created_at DESC`
		args = append(args, id.TeacherID)
	}
	rows, _ := db.Query(q, args...)
	var ids []int
	for rows.Next() {
		var cid int
		rows.Scan(&cid)
		ids = append(ids, cid)
	}
	rows.Close()
	out := []CourseView{}
	for _, cid := range ids {
		if c, ok := loadCourse(cid); ok {
			out = append(out, c)
		}
	}
	writeJSON(w, out)
}

func (h *APIHandler) TeacherCourseGet(w http.ResponseWriter, r *http.Request) {
	id, ok := requireAuth(w, r, "teacher", "admin")
	if !ok {
		return
	}
	var cid int
	fmt.Sscanf(r.URL.Query().Get("id"), "%d", &cid)
	c, found := loadCourse(cid)
	if !found || !ownsCourse(id, c) {
		http.Error(w, "Course not found", http.StatusNotFound)
		return
	}
	writeJSON(w, c)
}

func (h *APIHandler) TeacherCourseCreate(w http.ResponseWriter, r *http.Request) {
	id, ok := requireAuth(w, r, "teacher", "admin")
	if !ok {
		return
	}
	var req struct {
		Title    string `json:"title"`
		Syllabus string `json:"syllabus"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	title := strings.TrimSpace(req.Title)
	if title == "" {
		http.Error(w, "Course title required", http.StatusBadRequest)
		return
	}
	var cid int64
	for attempt := 0; attempt < 5; attempt++ {
		res, err := db.Exec(`INSERT INTO courses (teacher_id, title, syllabus, join_code) VALUES (?, ?, ?, ?)`,
			id.TeacherID, title, req.Syllabus, randomCode(6))
		if err == nil {
			cid, _ = res.LastInsertId()
			break
		}
	}
	if cid == 0 {
		http.Error(w, "Could not create course", http.StatusInternalServerError)
		return
	}
	objs, method := h.extractObjectives(title, req.Syllabus)
	saveObjectives(int(cid), objs)
	c, _ := loadCourse(int(cid))
	writeJSON(w, map[string]interface{}{"course": c, "extract_method": method})
}

func (h *APIHandler) TeacherCourseUpdate(w http.ResponseWriter, r *http.Request) {
	id, ok := requireAuth(w, r, "teacher", "admin")
	if !ok {
		return
	}
	var req struct {
		ID         int             `json:"id"`
		Title      string          `json:"title"`
		Syllabus   *string         `json:"syllabus"`
		Overrides  json.RawMessage `json:"overrides"`
		Objectives []string        `json:"objectives"`
		Reextract  bool            `json:"reextract"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	c, found := loadCourse(req.ID)
	if !found || !ownsCourse(id, c) {
		http.Error(w, "Course not found", http.StatusNotFound)
		return
	}
	if t := strings.TrimSpace(req.Title); t != "" {
		c.Title = t
	}
	if req.Syllabus != nil {
		c.Syllabus = *req.Syllabus
	}
	overrides := string(c.Overrides)
	if len(req.Overrides) > 0 {
		overrides = cleanOverrides(req.Overrides)
	}
	db.Exec(`UPDATE courses SET title = ?, syllabus = ?, settings = ?, updated_at = CURRENT_TIMESTAMP WHERE id = ?`,
		c.Title, c.Syllabus, overrides, c.ID)

	method := ""
	if req.Reextract {
		var objs []string
		objs, method = h.extractObjectives(c.Title, c.Syllabus)
		saveObjectives(c.ID, objs)
	} else if req.Objectives != nil {
		saveObjectives(c.ID, req.Objectives)
	}
	updated, _ := loadCourse(c.ID)
	writeJSON(w, map[string]interface{}{"course": updated, "extract_method": method})
}

// ==================== STUDENT: JOIN A COURSE ====================

func (h *APIHandler) CourseJoin(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Code        string `json:"code"`
		StudentName string `json:"student_name"`
		StudentKey  string `json:"student_key"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	if !keyRe.MatchString(req.StudentKey) {
		req.StudentKey = ""
	}
	var cid int
	err := db.QueryRow(`SELECT id FROM courses WHERE join_code = ?`, strings.ToUpper(strings.TrimSpace(req.Code))).Scan(&cid)
	c, found := loadCourse(cid)
	if err != nil || !found {
		http.Error(w, "No course has that code.", http.StatusNotFound)
		return
	}
	if len(c.Objectives) == 0 {
		http.Error(w, "This course has no learning objectives yet. Ask your teacher.", http.StatusConflict)
		return
	}
	sid := randomHex(16)
	sid = sid[:8] + "-" + sid[8:12] + "-" + sid[12:16] + "-" + sid[16:20] + "-" + sid[20:]
	_, err = db.Exec(`INSERT INTO sessions (id, student_name, subject, topic, status, course_id, student_key) VALUES (?, ?, ?, ?, 'profile', ?, ?)`,
		sid, req.StudentName, c.Title, c.Title, c.ID, req.StudentKey)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	for _, o := range c.Objectives {
		db.Exec(`INSERT INTO concepts (session_id, name) VALUES (?, ?)`, sid, o)
	}
	writeJSON(w, map[string]interface{}{
		"session_id": sid,
		"course": map[string]interface{}{
			"title":           c.Title,
			"objectives":      c.Objectives,
			"outputs":         c.Effective.Outputs,
			"paste_policy":    c.Effective.PastePolicy,
			"grasp_threshold": c.Effective.GraspThreshold,
		},
	})
}

// sessionSettings returns the effective settings and guidance for a session's course.
func sessionSettings(sessionID string) (Settings, string, int) {
	pol := loadPolicy()
	var courseID int
	db.QueryRow(`SELECT COALESCE(course_id, 0) FROM sessions WHERE id = ?`, sessionID).Scan(&courseID)
	var overrides string
	if courseID != 0 {
		db.QueryRow(`SELECT settings FROM courses WHERE id = ?`, courseID).Scan(&overrides)
	}
	s := effectiveSettings(pol, overrides)
	return s, tutorGuidance(pol, s), courseID
}

// ==================== SYLLABUS -> OBJECTIVES ====================

var bulletRe = regexp.MustCompile(`^\s*(?:[-*\x{2022}]|\d+[.)]|(?i:week|unit|chapter|module|lesson|topic)\s*\d*\s*[:.\-]?)\s*(.{8,160})$`)

// objectivesFromText is the offline fallback: bullet and numbered lines, else the first lines.
func objectivesFromText(title, syllabus string) []string {
	var out []string
	seen := map[string]bool{}
	add := func(s string) {
		s = strings.TrimSpace(s)
		if s != "" && !seen[strings.ToLower(s)] && len(out) < 12 {
			seen[strings.ToLower(s)] = true
			out = append(out, s)
		}
	}
	lines := strings.Split(syllabus, "\n")
	for _, ln := range lines {
		if m := bulletRe.FindStringSubmatch(ln); m != nil {
			add(m[1])
		}
	}
	if len(out) == 0 {
		for _, ln := range lines {
			if t := strings.TrimSpace(ln); len(t) >= 8 && len(t) <= 160 {
				add(t)
			}
		}
	}
	if len(out) == 0 && strings.TrimSpace(title) != "" {
		out = []string{title}
	}
	return out
}

// extractObjectives asks the model for objectives; with no model it falls back to text structure.
func (h *APIHandler) extractObjectives(title, syllabus string) ([]string, string) {
	if h.aiOn() && strings.TrimSpace(syllabus) != "" {
		if len(syllabus) > 12000 {
			syllabus = syllabus[:12000]
		}
		sys := "You turn a course syllabus into learning objectives for a Socratic tutor. Reply with ONLY a JSON array of strings."
		prompt := fmt.Sprintf(`Course: %s

Syllabus:
%s

Write one learning objective per distinct topic the syllabus lists, at most 12. Do not pad: a three-line syllabus gets about three objectives. Each covers exactly ONE idea, in one sentence a student could demonstrate by explaining in their own words (for example "Explain why supply curves slope upward"). Never join two ideas with "and". Cover the whole syllabus, in teaching order. Do not invent topics the syllabus does not mention.`, title, syllabus)
		if resp, err := h.ai("judge", sys, prompt); err == nil {
			if objs := parseJSONStrings(resp); len(objs) > 0 {
				return objs, "ai"
			}
		} else {
			log.Printf("objective extraction failed, using text fallback: %v", err)
		}
	}
	return objectivesFromText(title, syllabus), "text"
}

func parseJSONStrings(s string) []string {
	i, j := strings.Index(s, "["), strings.LastIndex(s, "]")
	if i < 0 || j <= i {
		return nil
	}
	var arr []string
	if json.Unmarshal([]byte(s[i:j+1]), &arr) != nil {
		return nil
	}
	var out []string
	for _, a := range arr {
		if a = strings.TrimSpace(a); a != "" && len(a) <= 300 && len(out) < 12 {
			out = append(out, a)
		}
	}
	return out
}

// ==================== GRASP JUDGE ====================

// judgeObjectives decides which still-open objectives the student's latest message demonstrates.
// It returns objective -> verbatim quote, and the method used ("ai" or "keyword").
func (h *APIHandler) judgeObjectives(subject string, open []string, history []map[string]string, message string) (map[string]string, string) {
	found := map[string]string{}
	if len(open) == 0 || strings.TrimSpace(message) == "" {
		return found, "none"
	}
	if !h.aiOn() {
		// ponytail: keyword guess, only when no model is configured. Teacher sees the method on each objective.
		for _, name := range evaluateConceptsAddressed(message, open) {
			found[name] = truncate(message, 160)
		}
		return found, "keyword"
	}

	if c, _ := resolveLLM(); c.Provider == "openai" {
		return h.judgeEach(subject, open, message), "ai"
	}

	var recent strings.Builder
	start := 0
	if len(history) > 3 {
		start = len(history) - 3
	}
	for _, e := range history[start:] {
		fmt.Fprintf(&recent, "Student: %s\nTutor: %s\n\n", e["human"], e["ai"])
	}
	var objs strings.Builder
	for i, o := range open {
		fmt.Fprintf(&objs, "%d. %s\n", i+1, o)
	}
	sys := `You are a strict grader of student understanding. Reply with ONLY a JSON array.`
	prompt := fmt.Sprintf(`Subject: %s

Open learning objectives:
%s
Recent dialogue:
%s
The student's LATEST message:
"""%s"""

For each objective, decide whether the latest message shows the student understands it. Count it ONLY if the student, in their own words, explains the idea, applies it to a new case, or corrects a misconception. Do NOT count: repeating the tutor's wording, restating the objective, vague agreement, or a guess. If unsure, do not count it.

Reply with a JSON array, one item per objective you count: [{"n": 1, "quote": "exact words copied from the latest message that show it"}]. Reply [] if none.`,
		subject, objs.String(), recent.String(), message)

	resp, err := h.ai("judge", sys, prompt)
	if err != nil {
		log.Printf("grasp judge failed: %v", err)
		return found, "error"
	}
	i, j := strings.Index(resp, "["), strings.LastIndex(resp, "]")
	if i < 0 || j <= i {
		return found, "ai"
	}
	var items []struct {
		N     int    `json:"n"`
		Quote string `json:"quote"`
	}
	if json.Unmarshal([]byte(resp[i:j+1]), &items) != nil {
		return found, "ai"
	}
	for _, it := range items {
		q := strings.TrimSpace(it.Quote)
		// the quote must really be the student's words, or the judgment is discarded
		if len(open) == 1 {
			it.N = 1 // one objective listed, so any number the model gave means that one
		}
		if it.N >= 1 && it.N <= len(open) && q != "" && strings.Contains(message, q) {
			found[open[it.N-1]] = q
		}
	}
	return found, "ai"
}

// ==================== TEACHER: ACCOMMODATION ====================

// SessionAccommodation marks a student as using assistive tools (dictation and the like), so
// capture signals stay on record but no longer lower their authenticity score.
func (h *APIHandler) SessionAccommodation(w http.ResponseWriter, r *http.Request) {
	id, ok := requireAuth(w, r, "teacher", "admin")
	if !ok {
		return
	}
	var req struct {
		ID string `json:"id"`
		On bool   `json:"on"`
	}
	if !readJSON(w, r, &req) {
		return
	}
	var owner, courseID int
	err := db.QueryRow(`SELECT COALESCE(s.course_id, 0), COALESCE(c.teacher_id, 0) FROM sessions s
		LEFT JOIN courses c ON c.id = s.course_id WHERE s.id = ?`, req.ID).Scan(&courseID, &owner)
	if err != nil || (id.Role == "teacher" && (courseID == 0 || owner != id.TeacherID)) {
		http.Error(w, "Session not found", http.StatusNotFound)
		return
	}
	v := 0
	if req.On {
		v = 1
	}
	db.Exec(`UPDATE sessions SET accommodation = ? WHERE id = ?`, v, req.ID)
	writeJSON(w, map[string]interface{}{"accommodation": v})
}
