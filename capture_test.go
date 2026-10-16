package main

import "testing"

func kinds(sigs []Signal) map[string]string {
	m := map[string]string{}
	for _, s := range sigs {
		m[s.Kind] = s.Level
	}
	return m
}

func TestCaptureSignals(t *testing.T) {
	long := "Scarcity means that resources are limited while wants are not, so people have to choose what to give up."
	earlier := []string{"My earlier answer: " + long, "Tutor: what do you mean by scarcity?"}

	// honestly typed: nothing to report
	typed := Capture{TypedChars: 130, Edits: 140, Deletions: 6, FinalLen: 130, ElapsedMs: 60000}
	if got := captureSignals(typed, earlier); len(got) != 0 {
		t.Fatalf("typed answer flagged: %v", got)
	}

	// mostly pasted from somewhere else
	pasted := Capture{PastedChars: 400, PasteCount: 1, FinalLen: 420, Edits: 20, Pasted: []string{string(make([]rune, 0)) + "Opportunity cost is the value of the next best alternative that you forgo when you make a choice among scarce options, which every economist defines this way."}}
	if k := kinds(captureSignals(pasted, earlier)); k["pasted_most"] != "review" {
		t.Fatalf("mostly pasted not flagged: %v", k)
	}

	// pasting the student's own earlier words, or the tutor's, is not held against them
	own := Capture{PastedChars: len(long), PasteCount: 1, FinalLen: 200, Edits: 80, Pasted: []string{long}}
	k := kinds(captureSignals(own, earlier))
	if _, bad := k["pasted_most"]; bad || k["pasted_own"] != "info" {
		t.Fatalf("own-words paste handled wrong: %v", k)
	}

	// a small paste in a mostly typed answer is context only
	part := Capture{PastedChars: 60, PasteCount: 1, FinalLen: 400, Edits: 300, Pasted: []string{"a sixty character quote from the reading that is long enough ok"}}
	if k := kinds(captureSignals(part, earlier)); k["pasted_part"] != "info" || k["pasted_most"] != "" {
		t.Fatalf("partial paste: %v", k)
	}

	// faster than typing, and arriving with no typing at all
	fast := Capture{FinalLen: 300, ElapsedMs: 5000, Edits: 12}
	if k := kinds(captureSignals(fast, nil)); k["fast_entry"] != "review" {
		t.Fatalf("fast entry: %v", k)
	}
	none := Capture{FinalLen: 200}
	if k := kinds(captureSignals(none, nil)); k["no_typing"] != "review" {
		t.Fatalf("no typing: %v", k)
	}

	// quotes placed by the paper builder are the student's own, not paste or speed
	paper := Capture{FinalLen: 900, InsertedChars: 500, TypedChars: 400, Edits: 380, ElapsedMs: 300000}
	if got := captureSignals(paper, nil); len(got) != 0 {
		t.Fatalf("inserted quotes flagged: %v", got)
	}

	// blocked paste attempts: one is info, three is review; away time is info
	blocked := Capture{PasteBlocked: 1, FinalLen: 100, Edits: 100, ElapsedMs: 90000, AwayMs: 45000}
	k = kinds(captureSignals(blocked, nil))
	if k["paste_attempts"] != "info" || k["left_page"] != "info" {
		t.Fatalf("blocked/away: %v", k)
	}
	if k := kinds(captureSignals(Capture{PasteBlocked: 3, FinalLen: 100, Edits: 100}, nil)); k["paste_attempts"] != "review" {
		t.Fatalf("repeated attempts: %v", k)
	}

	if d := captureDeduction([]Signal{{"pasted_most", "review", ""}, {"left_page", "info", ""}, {"fast_entry", "review", ""}}); d != 25 {
		t.Fatalf("deduction %d", d)
	}
}

func TestSanitize(t *testing.T) {
	c := Capture{PastedChars: -5, ElapsedMs: 1 << 40, Pasted: make([]string, 50)}
	c.sanitize()
	if c.PastedChars != 0 || c.ElapsedMs <= 0 || len(c.Pasted) != 10 {
		t.Fatalf("sanitize: %+v", c)
	}
}
