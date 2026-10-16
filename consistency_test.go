package main

import (
	"strings"
	"testing"
)

func TestNewTermsAndStyleJump(t *testing.T) {
	known := "scarcity means we cannot have everything. The tutor asked about opportunity cost and choices."
	own := []string{
		"i think scarcity is when there is not enough stuff for everyone so you pick",
		"like when i pick pizza i cant also get tacos with the same money",
		"so every choice costs me something else i wanted",
	}
	plain := "I think opportunity cost is what I give up when I pick one thing over another one, like pizza and tacos."
	if nt := newTerms(plain, known); len(nt) != 0 {
		t.Fatalf("ordinary answer flagged: %v", nt)
	}
	if _, ok := styleJump(plain, own); ok {
		t.Fatal("ordinary answer flagged as a style jump")
	}
	fed := "Opportunity cost represents the foregone utility of the next-best alternative, whereby rational agents systematically " +
		"internalize comparative disadvantages through marginal reallocation of heterogeneous endowments across intertemporal constraints."
	if nt := newTerms(fed, known); len(nt) < 4 {
		t.Fatalf("expected outside terms, got %v", nt)
	}
	if d, ok := styleJump(fed, own); !ok || !strings.Contains(d, "letters") && !strings.Contains(d, "words") {
		t.Fatalf("expected a style jump, got %q %v", d, ok)
	}
}

func TestConsistencySignalsUseSessionRecord(t *testing.T) {
	testDB(t)
	db.Exec(`INSERT INTO sessions (id, student_name, subject, topic, status) VALUES ('s1','Sam','Economics','Economics','learning')`)
	db.Exec(`INSERT INTO profiles (session_id, responses, fingerprint) VALUES ('s1', ?, '{}')`,
		`["i like fishing with my grandpa at the lake","school is ok i guess but math is hard","i want to be a vet someday maybe"]`)
	fed := "Opportunity cost represents the foregone utility of the next-best alternative, whereby rational agents systematically " +
		"internalize comparative disadvantages through marginal reallocation of heterogeneous endowments across intertemporal constraints."
	sigs := consistencySignals("s1", "dialogue", fed, Capture{MaxGapMs: 40000})
	kinds := map[string]string{}
	for _, s := range sigs {
		kinds[s.Kind] = s.Level
	}
	if kinds["new_terms"] != "review" || kinds["sophistication_jump"] != "review" || kinds["lookup_pattern"] != "review" {
		t.Fatalf("expected all three review signals, got %v", kinds)
	}
	if got := consistencySignals("s1", "dialogue", "short answer here", Capture{}); got != nil {
		t.Fatalf("short answers must not be judged: %v", got)
	}
}
