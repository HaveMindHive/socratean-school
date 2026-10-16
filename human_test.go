package main

import (
	"strings"
	"testing"
)

func TestHumanSigns(t *testing.T) {
	testDB(t)
	db.Exec(`INSERT INTO sessions (id, student_name, subject, topic, status) VALUES ('h1','Sam','Economics','Economics','learning')`)
	db.Exec(`INSERT INTO profiles (session_id, responses, fingerprint) VALUES ('h1', '["honestly the thing is i like fishing"]', '{"unique_patterns":["the thing is"]}')`)
	text := "I think scarcity is kind of like a pizza with too few slices, wait, actually it is when there is not enough for everyone. The thing is, my dad says money does not stretch. Why would that make us choose?"
	sigs := humanSigns("h1", text, Capture{TypedChars: 200, Deletions: 4, MaxGapMs: 6000}, nil)
	got := map[string]string{}
	for _, s := range sigs {
		if s.Level != "human" {
			t.Fatalf("human signs must be level human: %+v", s)
		}
		got[s.Kind] = s.Detail
	}
	for _, k := range []string{"hedging", "self_correction", "analogy", "personal", "own_question", "own_phrase", "revised", "paused"} {
		if got[k] == "" {
			t.Errorf("missing %s in %v", k, got)
		}
	}
	if !strings.Contains(got["analogy"], "like a pizza") || len(got["analogy"]) > 140 {
		t.Errorf("evidence should be a short quote: %q", got["analogy"])
	}
	// pasted-in text is not the student's thinking
	pasted := Capture{PastedChars: 200, PasteCount: 1, Pasted: []string{text}}
	for _, s := range humanSigns("h1", text, pasted, nil) {
		if s.Kind == "hedging" {
			t.Errorf("pasted text must not earn human signs: %+v", s)
		}
	}
	// plain, lifeless text earns none
	if got := humanSigns("h1", "Scarcity is the fundamental economic problem of limited resources against unlimited wants.", Capture{TypedChars: 90}, nil); len(got) != 0 {
		t.Errorf("flat text earned signs: %v", got)
	}
}
