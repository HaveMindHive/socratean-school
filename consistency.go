package main

// Consistency checks: does this answer fit what the student has shown so far? Anything a
// student writes is saved, so a sudden jump in vocabulary or sentence length, or long words
// that appeared nowhere in their earlier answers, the tutor's questions or the course
// material, can point to a second source (a phone, another person). These are hints for a
// teacher to weigh, never proof: students do learn words and do have good days.

import (
	"fmt"
	"regexp"
	"strings"
)

// tokRe matches every word, unlike wordRe which keeps only words of 4+ letters.
var tokRe = regexp.MustCompile(`[a-z]+`)

// knownMaterial is everything the student could legitimately have drawn on: their interview
// and earlier answers, the tutor's own words, and the course syllabus and objectives.
func knownMaterial(sessionID string) (own []string, known string) {
	own = studentInputs(sessionID)
	var sb strings.Builder
	for _, t := range sessionContext(sessionID) {
		sb.WriteString(t + "\n")
	}
	var courseID int
	var title string
	db.QueryRow(`SELECT COALESCE(course_id, 0), COALESCE(subject, '') FROM sessions WHERE id = ?`, sessionID).Scan(&courseID, &title)
	sb.WriteString(title + "\n")
	if courseID != 0 {
		var syl string
		db.QueryRow(`SELECT COALESCE(syllabus_text, '') FROM courses WHERE id = ?`, courseID).Scan(&syl)
		sb.WriteString(syl + "\n")
	}
	rows, _ := db.Query(`SELECT name FROM concepts WHERE session_id = ?`, sessionID)
	for rows.Next() {
		var n string
		rows.Scan(&n)
		sb.WriteString(n + "\n")
	}
	rows.Close()
	return own, sb.String()
}

// studentInputs are the student's own earlier words: interview answers, then dialogue answers.
func studentInputs(sessionID string) []string {
	var out []string
	var raw string
	db.QueryRow(`SELECT COALESCE(responses, '[]') FROM profiles WHERE session_id = ?`, sessionID).Scan(&raw)
	var resp []string
	jsonUnmarshal(raw, &resp)
	out = append(out, resp...)
	rows, _ := db.Query(`SELECT human_input FROM exchanges WHERE session_id = ? ORDER BY timestamp`, sessionID)
	for rows.Next() {
		var h string
		rows.Scan(&h)
		if strings.TrimSpace(h) != "" {
			out = append(out, h)
		}
	}
	rows.Close()
	return out
}

// newTerms lists long words in text whose stem appears nowhere in known.
func newTerms(text, known string) []string {
	have := stems(known)
	seen := map[string]bool{}
	var out []string
	for _, w := range wordRe.FindAllString(strings.ToLower(text), -1) {
		if len(w) < 10 {
			continue
		}
		s := w[:6]
		if have[s] || seen[s] {
			continue
		}
		seen[s] = true
		out = append(out, w)
	}
	return out
}

func wordStats(text string) (words int, avgLen, avgSentence float64) {
	text = strings.ToLower(text)
	ws := tokRe.FindAllString(text, -1)
	if len(ws) == 0 {
		return 0, 0, 0
	}
	letters := 0
	for _, w := range ws {
		letters += len(w)
	}
	sentences := 0
	for _, s := range strings.FieldsFunc(text, func(r rune) bool { return r == '.' || r == '!' || r == '?' }) {
		if len(tokRe.FindAllString(s, -1)) > 0 {
			sentences++
		}
	}
	if sentences == 0 {
		sentences = 1
	}
	return len(ws), float64(letters) / float64(len(ws)), float64(len(ws)) / float64(sentences)
}

// styleJump reports whether text is far more elaborate than the student's earlier writing.
func styleJump(text string, earlier []string) (string, bool) {
	if len(earlier) < 3 {
		return "", false
	}
	n, wl, sl := wordStats(text)
	bn, bwl, bsl := wordStats(strings.Join(earlier, ". "))
	if n < 25 || bn < 20 {
		return "", false
	}
	if bsl < 8 {
		bsl = 8 // very short earlier sentences should not make ordinary writing look like a jump
	}
	switch {
	case wl >= bwl+0.9:
		return fmt.Sprintf("Average word length is %.1f letters here against %.1f in their earlier answers.", wl, bwl), true
	case sl >= bsl*2.2 && sl >= 28:
		return fmt.Sprintf("Sentences average %d words here against %d in their earlier answers.", int(sl+0.5), int(bsl+0.5)), true
	}
	return "", false
}

// consistencySignals compares an answer with everything the student has already written.
// context is "dialogue", "synthesis" or "paper": longer writing legitimately brings in more
// new words, so those need more of them before anything is marked for review.
func consistencySignals(sessionID, context, text string, c Capture) []Signal {
	if n, _, _ := wordStats(text); n < 25 {
		return nil
	}
	own, known := knownMaterial(sessionID)
	// Pasting the student's own earlier words is allowed, so judge only what is new.
	nt := newTerms(text, known)
	need := 4
	if context != "dialogue" {
		need = 6
	}
	var out []Signal
	if len(nt) >= 2 {
		lvl := "info"
		if len(nt) >= need {
			lvl = "review"
		}
		shown := nt
		if len(shown) > 6 {
			shown = shown[:6]
		}
		out = append(out, Signal{"new_terms", lvl, fmt.Sprintf(
			"Used %d long words that appear nowhere in their earlier answers, the tutor's questions or the course material: %s.",
			len(nt), strings.Join(shown, ", "))})
	}
	jumped := false
	if detail, ok := styleJump(text, own); ok {
		jumped = true
		out = append(out, Signal{"sophistication_jump", "review", "Writing is more elaborate than the student's earlier answers. " + detail})
	}
	if c.MaxGapMs >= 30000 && (len(nt) >= 2 || jumped) {
		out = append(out, Signal{"lookup_pattern", "review", fmt.Sprintf(
			"Stopped typing for about %d seconds, then the text brought in words or elaboration not seen earlier. That may mean looking something up, or just thinking.", c.MaxGapMs/1000)})
	}
	return out
}
