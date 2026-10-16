package main

import "testing"

func TestLearningStyleReadsInputs(t *testing.T) {
	testDB(t)
	db.Exec(`INSERT INTO sessions (id, student_name, subject, topic, status) VALUES ('l1','Sam','Econ','Econ','learning')`)
	few := `["it is like a pizza, for example","my dad says so","ok"]`
	db.Exec(`INSERT INTO profiles (session_id, responses, fingerprint) VALUES ('l1', ?, '{}')`, few)
	if r := learningStyle("l1"); len(r.Names) != 0 {
		t.Fatalf("too few answers should say nothing: %+v", r)
	}
	db.Exec(`INSERT INTO exchanges (session_id, human_input, ai_response) VALUES ('l1','imagine a lemonade stand, for example','x')`)
	db.Exec(`INSERT INTO exchanges (session_id, human_input, ai_response) VALUES ('l1','my mom and I do that when we shop, it is like a budget','x')`)
	r := learningStyle("l1")
	if len(r.Names) == 0 || r.Names[0] != "examples and analogies" || r.Hint == "" {
		t.Fatalf("expected examples first: %+v", r)
	}
}

func TestStatusEarnedAndHeld(t *testing.T) {
	testDB(t)
	if updateStatus("", true) != nil {
		t.Fatal("no key, no status")
	}
	k := "abcdef0123456789"
	s := updateStatus(k, true)
	if s["tier"] != "Witnessed Thinker" || s["held"] != 1 {
		t.Fatalf("first witnessed work: %v", s)
	}
	updateStatus(k, true)
	s = updateStatus(k, true)
	if s["tier"] != "Witnessed Scholar" || s["held"] != 3 {
		t.Fatalf("three works: %v", s)
	}
	s = updateStatus(k, false) // an unwitnessed work never takes the status away
	if s["tier"] != "Witnessed Scholar" || s["held"] != 0 || s["works"] != 3 {
		t.Fatalf("status must survive a lapse: %v", s)
	}
	if !isWitnessed(85, 95, true, "some") || isWitnessed(60, 95, true, "strong") || isWitnessed(85, 40, true, "strong") || isWitnessed(85, 95, true, "little") {
		t.Fatal("witnessed rules wrong")
	}
	if !keyRe.MatchString(k) || keyRe.MatchString("../etc") {
		t.Fatal("key validation")
	}
}
