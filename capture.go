package main

// Capture records HOW an answer was written (typing, pasting, time away) so a teacher has
// facts, not a verdict. The browser reports these; a determined student can forge them, so
// they are evidence for a teacher to weigh, never proof. Signals describe, they do not accuse.

import (
	"encoding/json"
	"fmt"
	"strings"
)

type Capture struct {
	TypedChars    int      `json:"typed_chars"`
	PastedChars   int      `json:"pasted_chars"`
	PasteCount    int      `json:"paste_count"`
	PasteBlocked  int      `json:"paste_blocked"`
	InsertedChars int      `json:"inserted_chars"` // the student's own saved quotes, placed by the paper builder
	Edits         int      `json:"edits"`          // input events (works for soft keyboards, unlike key codes)
	Deletions     int      `json:"deletions"`
	FinalLen      int      `json:"final_len"`
	ElapsedMs     int      `json:"elapsed_ms"`
	AwayMs        int      `json:"away_ms"`
	AwayCount     int      `json:"away_count"`
	MaxGapMs      int      `json:"max_gap_ms"` // longest pause between keystrokes while on the page
	Pasted        []string `json:"pasted"`
}

type Signal struct {
	Kind   string `json:"kind"`
	Level  string `json:"level"` // "review" (worth a look) or "info" (context only)
	Detail string `json:"detail"`
}

func clampInt(v, max int) int {
	if v < 0 {
		return 0
	}
	if v > max {
		return max
	}
	return v
}

// sanitize bounds everything the client sent.
func (c *Capture) sanitize() {
	const big = 10000000
	c.TypedChars, c.PastedChars = clampInt(c.TypedChars, big), clampInt(c.PastedChars, big)
	c.PasteCount, c.PasteBlocked = clampInt(c.PasteCount, 10000), clampInt(c.PasteBlocked, 10000)
	c.InsertedChars, c.Edits = clampInt(c.InsertedChars, big), clampInt(c.Edits, big)
	c.Deletions, c.FinalLen = clampInt(c.Deletions, big), clampInt(c.FinalLen, big)
	c.ElapsedMs, c.AwayMs = clampInt(c.ElapsedMs, 1<<31-1), clampInt(c.AwayMs, 1<<31-1)
	c.AwayCount = clampInt(c.AwayCount, 10000)
	c.MaxGapMs = clampInt(c.MaxGapMs, 1<<31-1)
	if len(c.Pasted) > 10 {
		c.Pasted = c.Pasted[:10]
	}
	for i, p := range c.Pasted {
		if r := []rune(p); len(r) > 2000 {
			c.Pasted[i] = string(r[:2000])
		}
	}
}

func norm(s string) string { return strings.ToLower(strings.Join(strings.Fields(s), " ")) }

// reused reports whether a pasted snippet is the student's own earlier words or the tutor's text.
func reused(snippet string, texts []string) bool {
	s := norm(snippet)
	if s == "" {
		return true
	}
	for _, t := range texts {
		if strings.Contains(norm(t), s) {
			return true
		}
	}
	return false
}

// captureSignals turns raw capture facts into signals. earlier = the student's own earlier
// answers and the tutor's messages, so copying those is not counted against the student.
func captureSignals(c Capture, earlier []string) []Signal {
	var out []Signal
	external, internal, sum := 0, 0, 0
	for _, sn := range c.Pasted {
		n := len([]rune(sn))
		sum += n
		if reused(sn, earlier) {
			internal += n
		} else {
			external += n
		}
	}
	if c.PastedChars > sum {
		external += c.PastedChars - sum // pastes beyond the stored snippets
	}
	own := c.FinalLen - c.InsertedChars // length the student was responsible for writing
	if own < 1 {
		own = 1
	}
	ratio := float64(external) / float64(own)
	if ratio > 1 {
		ratio = 1
	}

	if external >= 40 {
		if ratio >= 0.6 {
			out = append(out, Signal{"pasted_most", "review",
				fmt.Sprintf("About %d%% of this answer (%d of %d characters) was pasted in, not typed.", int(ratio*100+0.5), external, own)})
		} else {
			out = append(out, Signal{"pasted_part", "info",
				fmt.Sprintf("%d characters were pasted in. The rest was typed.", external)})
		}
	}
	if internal >= 40 {
		out = append(out, Signal{"pasted_own", "info",
			fmt.Sprintf("Pasted %d characters that were the student's own earlier words or the tutor's text. Not counted against them.", internal)})
	}
	if c.PasteBlocked > 0 {
		lvl := "info"
		if c.PasteBlocked >= 3 {
			lvl = "review"
		}
		out = append(out, Signal{"paste_attempts", lvl,
			fmt.Sprintf("Tried to paste %d time(s) text that was not their own earlier words. It was refused, so nothing outside was pasted.", c.PasteBlocked)})
	}
	if own >= 120 && c.ElapsedMs >= 1000 && ratio < 0.3 {
		cps := float64(own) / (float64(c.ElapsedMs) / 1000)
		if cps >= 15 {
			out = append(out, Signal{"fast_entry", "review",
				fmt.Sprintf("Text appeared at about %d characters per second, faster than typing. Dictation and some assistive tools can do this too.", int(cps+0.5))})
		}
	}
	if own >= 80 && c.Edits == 0 && external == 0 && c.PasteCount == 0 {
		out = append(out, Signal{"no_typing", "review", "This answer arrived with no typing or pasting recorded."})
	}
	if c.AwayMs >= 30000 && c.FinalLen >= 80 {
		out = append(out, Signal{"left_page", "info",
			fmt.Sprintf("Left the page for about %d seconds while writing. That may mean looking something up, or just another tab.", c.AwayMs/1000)})
	}
	return out
}

// captureDeduction is how much the review-level signals lower the authenticity score.
func captureDeduction(sigs []Signal) int {
	d := 0
	for _, s := range sigs {
		if s.Level != "review" {
			continue
		}
		switch s.Kind {
		case "pasted_most":
			d += 15
		case "fast_entry", "no_typing":
			d += 10
		case "paste_attempts":
			d += 5
		}
	}
	return d
}

func countReview(sigs []Signal) int {
	n := 0
	for _, s := range sigs {
		if s.Level == "review" {
			n++
		}
	}
	return n
}

// externalPasted mirrors captureSignals' split, for the "written by typing" share.
func externalPasted(c Capture, earlier []string) int {
	ext, sum := 0, 0
	for _, sn := range c.Pasted {
		n := len([]rune(sn))
		sum += n
		if !reused(sn, earlier) {
			ext += n
		}
	}
	if c.PastedChars > sum {
		ext += c.PastedChars - sum
	}
	return ext
}

// sessionContext returns the student's own earlier answers plus the tutor's messages.
func sessionContext(sessionID string) []string {
	var out []string
	var raw string
	db.QueryRow(`SELECT COALESCE(responses, '[]') FROM profiles WHERE session_id = ?`, sessionID).Scan(&raw)
	var resp []string
	jsonUnmarshal(raw, &resp)
	out = append(out, resp...) // the interview answers are the student's own words too
	rows, _ := db.Query(`SELECT human_input, ai_response FROM exchanges WHERE session_id = ?`, sessionID)
	for rows.Next() {
		var h, a string
		rows.Scan(&h, &a)
		out = append(out, h, a)
	}
	rows.Close()
	return out
}

// recordCapture stores one answer's capture with its derived signals. Call it BEFORE the
// answer itself is saved, so a paste is compared to earlier text only.
func recordCapture(sessionID, context string, exchangeNum int, c *Capture, text string) []Signal {
	if c == nil {
		return nil
	}
	c.sanitize()
	earlier := sessionContext(sessionID)
	sigs := captureSignals(*c, earlier)
	if text != "" {
		sigs = append(sigs, consistencySignals(sessionID, context, text, *c)...)
		sigs = append(sigs, humanSigns(sessionID, text, *c, earlier)...)
	}
	summary, _ := json.Marshal(c)
	sj, _ := json.Marshal(sigs)
	db.Exec(`INSERT INTO capture_events (session_id, context, exchange_num, summary, signals, review_count, ext_pasted, final_len, inserted_len)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		sessionID, context, exchangeNum, string(summary), string(sj), countReview(sigs),
		externalPasted(*c, earlier), c.FinalLen, c.InsertedChars)
	return sigs
}

const captureSchema = `
	CREATE TABLE IF NOT EXISTS capture_events (
		id INTEGER PRIMARY KEY AUTOINCREMENT,
		session_id TEXT,
		context TEXT,
		exchange_num INTEGER,
		summary TEXT,
		signals TEXT,
		review_count INTEGER DEFAULT 0,
		ext_pasted INTEGER DEFAULT 0,
		final_len INTEGER DEFAULT 0,
		inserted_len INTEGER DEFAULT 0,
		created_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

// typedShare is the percent of what the student wrote that was typed rather than pasted in.
func typedShare(sessionID string) (int, bool) {
	var ext, total int
	db.QueryRow(`SELECT COALESCE(SUM(ext_pasted),0), COALESCE(SUM(MAX(final_len - inserted_len, 0)),0)
		FROM capture_events WHERE session_id = ?`, sessionID).Scan(&ext, &total)
	if total <= 0 {
		return 0, false
	}
	if ext > total {
		ext = total
	}
	return 100 - ext*100/total, true
}

// computeAuthenticity combines capture signals (strong, factual) with the old text signals
// (weak, capped so they cannot sink an honest student on their own).
func computeAuthenticity(sessionID string, aiScore, styleMatch float64) int {
	score := 100

	var accommodation int
	db.QueryRow(`SELECT COALESCE(accommodation, 0) FROM sessions WHERE id = ?`, sessionID).Scan(&accommodation)
	if accommodation == 0 {
		rows, _ := db.Query(`SELECT signals FROM capture_events WHERE session_id = ?`, sessionID)
		capDed := 0
		for rows.Next() {
			var sj string
			var sigs []Signal
			rows.Scan(&sj)
			json.Unmarshal([]byte(sj), &sigs)
			capDed += captureDeduction(sigs)
		}
		rows.Close()
		if capDed > 60 {
			capDed = 60
		}
		score -= capDed
	}

	var aiW, styleW int
	db.QueryRow(`SELECT COALESCE(SUM(type = 'ai_detected'),0), COALESCE(SUM(type = 'style_mismatch'),0) FROM warnings WHERE session_id = ?`, sessionID).Scan(&aiW, &styleW)
	score -= min(aiW*3, 15) + min(styleW*3, 10)
	score -= int(aiScore*10) + int((1-styleMatch)*8) // their opening voice matters: it is the baseline

	if score < 0 {
		score = 0
	}
	return score
}

func jsonUnmarshal(raw string, v interface{}) { json.Unmarshal([]byte(raw), v) }
