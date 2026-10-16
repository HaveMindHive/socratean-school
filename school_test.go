package main

import "testing"

func TestSettingsHierarchy(t *testing.T) {
	pol := SchoolPolicy{
		Defaults: Settings{GraspThreshold: 75, PastePolicy: "block", Outputs: []string{"paper"}},
		Locked:   []string{"paste_policy"},
		Guidance: "Be kind.",
	}
	// teacher tries to loosen a locked key and tighten an unlocked one
	s := effectiveSettings(pol, `{"paste_policy":"record","grasp_threshold":90,"outputs":["paper","test"],"reading_level":"grade 9"}`)
	if s.PastePolicy != "block" {
		t.Fatalf("locked key was overridden: %q", s.PastePolicy)
	}
	if s.GraspThreshold != 90 || len(s.Outputs) != 2 || s.ReadingLevel != "grade 9" {
		t.Fatalf("unlocked overrides lost: %+v", s)
	}
	// nonsense is clamped back to defaults
	s = effectiveSettings(pol, `{"grasp_threshold":3,"outputs":["exam"]}`)
	if s.GraspThreshold != 80 || len(s.Outputs) != 2 {
		t.Fatalf("bad values not normalized: %+v", s)
	}
	if g := tutorGuidance(pol, Settings{TutorGuidance: "Use Swedish examples."}); g != "School guidance: Be kind.\nCourse guidance: Use Swedish examples." {
		t.Fatalf("guidance order wrong: %q", g)
	}
}

func TestObjectivesFromText(t *testing.T) {
	syl := "Intro to Economics\nWeek 1: Scarcity and choice\nWeek 2: Supply and demand\n- Elasticity of demand\n3. Market failure and externalities\n"
	got := objectivesFromText("Econ", syl)
	want := []string{"Scarcity and choice", "Supply and demand", "Elasticity of demand", "Market failure and externalities"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("got %v", got)
		}
	}
	if got := objectivesFromText("Just a title", ""); len(got) != 1 || got[0] != "Just a title" {
		t.Fatalf("title fallback: %v", got)
	}
}

func TestPasscode(t *testing.T) {
	salt := randomHex(8)
	h := hashPass(salt, "ABCD2345EF")
	if !passMatches(salt, h, "ABCD2345EF") || passMatches(salt, h, "ABCD2345EG") {
		t.Fatal("passcode compare broken")
	}
}
