package main

// How a student learns, read from how they write. Every input is kept, and the way they
// reach for ideas shows what helps them: examples, their own life, pictures, ordered steps,
// asking why, trying numbers. The tutor leads its teaching with their best way in, and the
// student sees it as a strength. Patterns only, English only, and a guide not a label.

import (
	"fmt"
	"regexp"
	"sort"
	"strings"
)

type styleDef struct {
	key, name, hint string
	re              *regexp.Regexp
}

var styleDefs = []styleDef{
	{"examples", "examples and analogies", "lead with a concrete everyday example or an analogy before naming the idea",
		regexp.MustCompile(`(?i)\b(for example|for instance|such as|kind of like|sort of like|it'?s like|like a|imagine|say you)\b`)},
	{"story", "stories from their own life", "tie each idea to something from their own life and experience",
		regexp.MustCompile(`(?i)\b(my (mom|dad|mother|father|brother|sister|friend|friends|teacher|grandma|grandpa|family|job)|when i was|i remember|last (week|year|summer)|i (once|always|usually))\b`)},
	{"visual", "pictures and diagrams", "describe the idea as a picture, shape, map or diagram they can see in their head",
		regexp.MustCompile(`(?i)\b(picture|looks? like|draw|diagram|graph|shape|map|colou?r|visuali[sz]e|in my head i see)\b`)},
	{"steps", "step-by-step reasoning", "walk through it in clear ordered steps, then ask them to carry the chain one step further",
		regexp.MustCompile(`(?i)\b(first|then|next|step|because|so that|therefore|which means|that means)\b`)},
	{"why", "asking why and what if", "start from a puzzle or a what-if and let them work toward the idea",
		regexp.MustCompile(`(?i)\b(why|what if|how come|does that mean|what happens)\b`)},
	{"doing", "trying it out", "give them something small to try or work out with real numbers before explaining",
		regexp.MustCompile(`(?i)(\$\d|\b\d+ (dollars|bucks|hours|minutes)\b|\bi tried\b|\blet me try\b|\bplug\b|\bcalculate)`)},
}

type StyleResult struct {
	Names   []string `json:"names"`
	Keys    []string `json:"keys"`
	Answers int      `json:"answers"`
	Hint    string   `json:"-"` // for the tutor only
}

// learningStyle needs a few answers before it says anything. Interview answers count too.
func learningStyle(sessionID string) StyleResult {
	inputs := studentInputs(sessionID)
	res := StyleResult{Answers: len(inputs)}
	if len(inputs) < 4 {
		return res
	}
	type score struct {
		def styleDef
		n   int
	}
	var sc []score
	for _, d := range styleDefs {
		n := 0
		for _, in := range inputs {
			if d.re.MatchString(in) {
				n++
			}
		}
		if n >= 2 && n*4 >= len(inputs) {
			sc = append(sc, score{d, n})
		}
	}
	sort.SliceStable(sc, func(i, j int) bool { return sc[i].n > sc[j].n })
	if len(sc) > 2 {
		sc = sc[:2]
	}
	var hints []string
	for _, s := range sc {
		res.Names = append(res.Names, s.def.name)
		res.Keys = append(res.Keys, s.def.key)
		hints = append(hints, s.def.hint)
	}
	if len(sc) > 0 {
		res.Hint = fmt.Sprintf("From how this student writes, they take ideas in best through %s. When you teach, %s. Still end with one question that makes them use the idea.",
			strings.Join(res.Names, " and "), strings.Join(hints, "; "))
	}
	return res
}

// ==================== STATUS: earn it, hold it ====================

// A witnessed work is one where the thinking is on the record: the idea was reached, the
// writing was typed, and the evidence of a person thinking is there. Earning it is
// celebrated; a work that is not witnessed never takes a status away, it just pauses the run
// until the next one.

var tiers = []struct {
	works int
	name  string
}{{1, "Witnessed Thinker"}, {3, "Witnessed Scholar"}, {6, "Witnessed Mind"}}

const statusSchema = `
	CREATE TABLE IF NOT EXISTS scholars (
		student_key TEXT PRIMARY KEY,
		works INTEGER DEFAULT 0,
		held INTEGER DEFAULT 0,
		updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
	);`

var keyRe = regexp.MustCompile(`^[a-f0-9]{8,40}$`)

func tierFor(works int) (name, next string, toNext int) {
	for _, t := range tiers {
		if works >= t.works {
			name = t.name
		} else if next == "" {
			next, toNext = t.name, t.works-works
		}
	}
	return
}

func isWitnessed(score, typedPct int, haveTyped bool, humanLabel string) bool {
	return score >= 70 && (!haveTyped || typedPct >= 80) && humanLabel != "little"
}

// updateStatus records one finished work for the student and describes where they stand.
func updateStatus(studentKey string, witnessed bool) map[string]interface{} {
	if studentKey == "" {
		return nil
	}
	var works, held int
	db.QueryRow(`SELECT works, held FROM scholars WHERE student_key = ?`, studentKey).Scan(&works, &held)
	if witnessed {
		works++
		held++
	} else {
		held = 0
	}
	db.Exec(`INSERT OR REPLACE INTO scholars (student_key, works, held, updated_at) VALUES (?, ?, ?, CURRENT_TIMESTAMP)`, studentKey, works, held)
	name, next, toNext := tierFor(works)
	msg := ""
	switch {
	case witnessed && works == 1:
		msg = "You earned it. Every word of this is yours, and it is on the record."
	case witnessed:
		msg = fmt.Sprintf("Held. That is %d witnessed works in a row.", held)
	case name != "":
		msg = "Your status is safe. Your teacher will go over this with you, and your next witnessed work builds your run back."
	default:
		msg = "Your next work can earn your first status. Think it out loud and it will show."
	}
	return map[string]interface{}{"tier": name, "works": works, "held": held, "next": next, "to_next": toNext, "witnessed": witnessed, "message": msg}
}
