package main

// Signs of a person thinking: the signature method. A person working an idea out shows it:
// hedging, changing their mind mid-thought, reaching for an analogy, tying it to their own
// life, asking their own question, backspacing and rewording, pausing and carrying on, using
// the same turns of phrase they used in their opening interview, building on their own
// earlier words. Socratean records these as positive evidence next to the exact words that
// show them. They support a teacher's judgment and are not proof: text can be coached and
// the markers here are English only.

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type humanMarker struct {
	kind, label string
	re          *regexp.Regexp
}

var humanMarkers = []humanMarker{
	{"hedging", "Thinking out loud", regexp.MustCompile(`(?i)\b(i think|i guess|i suppose|i wonder|maybe|probably|kind of|sort of|not sure|or something|seems like)\b`)},
	{"self_correction", "Changed or reworked their own idea", regexp.MustCompile(`(?i)\b(wait|hold on|actually|i mean|oh,? so|oh!|hmm+|i was wrong|scratch that|let me rethink|no,? that'?s not)\b`)},
	{"analogy", "Reached for an analogy", regexp.MustCompile(`(?i)\b(kind of like|sort of like|it'?s like|like a|reminds me|same as|as if|imagine)\b`)},
	{"personal", "Tied it to their own life", regexp.MustCompile(`(?i)\b(my (mom|dad|mother|father|brother|sister|friend|friends|teacher|grandma|grandpa|grandfather|grandmother|dog|cat|family|job|boss)|when i was|i remember|last (week|year|summer)|where i (live|work))\b`)},
	{"own_question", "Asked a question of their own", regexp.MustCompile(`(?i)\b(why|how|what if|does that mean|so does|is it|are they)\b[^.?!]{3,80}\?`)},
	{"unsure", "Said what they were unsure of", regexp.MustCompile(`(?i)\b(i don'?t (get|understand|know)|confus\w+|doesn'?t make sense|i'?m lost|i'?m stuck|not clear to me)\b`)},
}

// clause returns a short stretch of the student's own words around a match: about 30
// characters before and 45 after, cut at word edges, so the teacher sees what they said.
func clause(text string, loc []int) string {
	start, end := loc[0]-30, loc[1]+45
	if start < 0 {
		start = 0
	}
	if end > len(text) {
		end = len(text)
	}
	for start > 0 && start < loc[0] && text[start-1] != ' ' {
		start++
	}
	for end < len(text) && end > loc[1] && text[end] != ' ' {
		end--
	}
	out := strings.TrimSpace(text[start:end])
	if start > 0 {
		out = "\u2026" + out
	}
	if end < len(text) {
		out += "\u2026"
	}
	return out
}

func humanSigns(sessionID, text string, c Capture, earlier []string) []Signal {
	var out []Signal
	add := func(kind, detail string) { out = append(out, Signal{kind, "human", detail}) }

	// What the words show. Skipped when much of the text was pasted in from outside.
	if externalPasted(c, earlier) < 40 && len(strings.Fields(text)) >= 6 {
		for _, m := range humanMarkers {
			if loc := m.re.FindStringIndex(text); loc != nil {
				add(m.kind, fmt.Sprintf("%s: “%s”", m.label, clause(text, loc)))
			}
		}
		// Phrases from their opening interview showing up again in this answer.
		var fpJSON string
		db.QueryRow(`SELECT COALESCE(fingerprint, '{}') FROM profiles WHERE session_id = ?`, sessionID).Scan(&fpJSON)
		var fp WritingFingerprint
		json.Unmarshal([]byte(fpJSON), &fp)
		low := strings.ToLower(text)
		for _, p := range fp.UniquePatterns {
			if p = strings.ToLower(strings.TrimSpace(p)); len(p) >= 4 && strings.Contains(low, p) {
				add("own_phrase", fmt.Sprintf("Used a phrase from their opening interview: “%s”", p))
				break
			}
		}
		// Building on their own earlier words.
		prior := stems(strings.Join(studentInputs(sessionID), " "))
		for _, w := range wordRe.FindAllString(low, -1) {
			if len(w) >= 7 && prior[w[:6]] {
				add("callback", fmt.Sprintf("Built on their own earlier words: “%s”", w))
				break
			}
		}
	}

	// How it was written.
	if c.Deletions >= 3 && c.TypedChars >= 30 {
		add("revised", fmt.Sprintf("Backspaced and reworded while writing (%d deletions).", c.Deletions))
	}
	if c.MaxGapMs >= 3000 && c.MaxGapMs < 30000 && c.TypedChars >= 40 && c.PastedChars == 0 {
		add("paused", fmt.Sprintf("Paused to think (longest %d seconds) and carried on.", c.MaxGapMs/1000))
	}
	return out
}

// humanEvidence summarizes the signs across a whole session for the teacher and certificate.
func humanEvidence(sessionID string) map[string]interface{} {
	rows, _ := db.Query(`SELECT context, signals FROM capture_events WHERE session_id = ? AND context != 'profile'`, sessionID)
	answers, with := 0, 0
	kinds := map[string]int{}
	for rows.Next() {
		var ctx, sj string
		var sigs []Signal
		rows.Scan(&ctx, &sj)
		json.Unmarshal([]byte(sj), &sigs)
		answers++
		n := 0
		for _, s := range sigs {
			if s.Level == "human" {
				kinds[s.Kind]++
				n++
			}
		}
		if n > 0 {
			with++
		}
	}
	rows.Close()
	label := "none recorded"
	switch {
	case answers == 0:
	case answers < 3:
		label = "too early" // a verdict on one or two answers would mean nothing
	case with*10 >= answers*6 && len(kinds) >= 4:
		label = "strong"
	case with*10 >= answers*3 && len(kinds) >= 2:
		label = "some"
	default:
		label = "little"
	}
	names := []string{}
	for k := range kinds {
		names = append(names, k)
	}
	sort.Strings(names)
	return map[string]interface{}{"label": label, "answers": answers, "with_signs": with, "kinds": kinds, "kind_names": names}
}
