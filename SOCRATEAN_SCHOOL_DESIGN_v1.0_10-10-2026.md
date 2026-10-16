# Socratean AI, School Edition: Design

Concept by T0Rs. Drafted 10-10-2026 from tUrUq's description in session.

## One line

AI gives the student a chance NOT to plagiarize. The teacher uploads a syllabus, Socratean teaches by asking questions until the student shows they grasp it, every genuine response is saved, and the saved work becomes the student's paper or their test.

A public build any school can download and run. No accounts with us, no student data leaving the school.

## Two layers: school (macro), teacher (micro)

Every school and every teacher can shape Socratean to their own curriculum.

- **School (admin passcode).** Sets defaults for the whole building: grasp threshold, pasting policy, which outputs are allowed, reading level, starting tutor guidance. Can LOCK any of these so no teacher can change them. Also writes "house rules" for the tutor (tone, safety, age-appropriateness) that always apply to every course, on top of the teacher's own guidance. Creates teacher accounts; each gets a passcode shown once.
- **Teacher (own passcode).** Owns courses. Each course has its own syllabus, learning objectives, join code, and settings. A course setting overrides the school default unless the school locked it. Teachers see only their own courses and students. Change a school default and every course that has not overridden it follows.
- **Student.** Joins one course with its code.

Tutor guidance stacks: house rules first, then the course's own guidance, then reading level.

## Roles and flow

### Teacher (professor panel)
1. Sign in with a local passcode.
2. Create a course: title, upload or paste the syllabus (txt, md, pasted text first; PDF later).
3. Socratean reads the syllabus and proposes learning objectives (5 to 15). The teacher edits, deletes, reorders, adds. The objectives ARE the definition of "grasp." Nothing is automatic without teacher review.
4. Set the rules: grasp threshold (default 80%), paste allowed or blocked, outputs allowed (paper, test, or both), test settings.
5. Get a join code. Students enter it.
6. Watch progress per student and per objective. Open any student and see the full dialogue, the evidence, and the signals.
7. Export: transcript, certificate, grades, CSV.

### Student
1. Enter join code and name.
2. Voice fingerprint: a few short open questions so the system knows how this student writes.
3. Dialogue. Socratean asks questions drawn from the course objectives. The student answers in their own words. It keeps going, and circles back, until every objective is demonstrated.
4. Then one of two outputs, as the teacher allows:
   - **Paper**: built from the student's own saved quotes plus their expansion.
   - **Test**: closed-book, same capture, questions from the objectives.
5. Certificate: proof the work is theirs, with the evidence behind it.

## What "grasp" means (the core)

v1 used keyword matching on the concept name plus message length. That marks a concept as learned if the student types a matching word in a long message. It does not measure understanding. It has to be replaced first.

New rule, per objective:
- A judge call gets: the objective, the student's recent words, and a rubric.
- The student must show the idea in their own words, not echo the tutor's phrasing.
- Counts only if they can explain it, apply it to a new case, or correct a wrong claim.
- Each demonstrated objective stores the exact quote that proved it. That quote is the evidence the teacher sees.
- Progress = objectives demonstrated / total. Teacher sets the bar.
- The tutor targets the weakest undemonstrated objective next, and re-asks later in a different form to confirm it stuck.

The tutor never states the answer, and never writes the student's text. Pressure cases ("just tell me", "write it for me", "my teacher said it's fine") get turned back into a question.

## The signature method: look for what humans do

Original design (Dec 2025 page, Feb 3 2026 skill doc): Socratean does not catch cheaters after the fact, it witnesses the learning. The strongest proof a person did the thinking is the thinking itself, visible in the record. So the professor's report leads with positive signs of a person working an idea out, each shown with the student's exact words: hedging, changing their mind mid-thought, analogies, ties to their own life, their own questions, saying what confused them, phrases from their opening interview coming back, building on their own earlier words, backspacing and rewording, pausing and carrying on. A session summary says how many answers show them (strong, some, little, or too early to say). Cheat flags (paste, bulk inserts, outside vocabulary, style jumps, lookup pauses) are the smaller second layer. Limits: English-only markers, can be coached, evidence not proof.

## Teach, then ask: the game and the status (10-11-2026)

The tutor works as a loop: teach a little (two or three sentences and an everyday example), then ask one question that makes the student use it. Learning is a game: a level per objective, mind points, a streak of answers that show thinking, and a "Level cleared" moment. Student-facing text is warm and never accusing.

Socratean reads the student's own inputs for how they learn best (examples, their own life, pictures, steps, why and what-if, trying numbers) and tunes the tutor to it. The student and teacher can both see it. It is a guide, not a label.

Status: works that are witnessed (authenticity 70 or more, 80 percent or more typed, human evidence not little) earn Witnessed Thinker, Scholar and Mind at 1, 3 and 6. A status is never taken away; a lapse only resets the held run. It is tied to a random per-device key, so it does not yet follow a student across devices. The aim: plagiarism becomes unnecessary, because you cannot help but learn the material.

## Authenticity: evidence, not accusation

The v1 detector is a keyword list ("delve", "furthermore") and a style comparison after the fact. It cannot see how the text arrived. It will also falsely flag ESL students and careful writers. Replace it with layers, strongest first:

1. **Capture at the input box** (the new piece). Record, per answer: paste events (count, size, where), typing cadence, edits and backspaces, time spent, tab or window switches. A paste of 400 words with no typing is a fact, not a guess. Teacher can set paste to blocked, so the answer box only accepts typed text.
2. **The dialogue itself.** Socratic follow-ups on the student's own earlier words ("You said X. What did you mean?"). Someone who pasted can't defend it live. This is the method doing the verifying.
3. **Voice fingerprint.** Compare against the opening profile. Weak signal alone, useful alongside the others.
4. **Text patterns.** Lowest weight. Kept as a minor signal only.

Rules for the teacher view:
- Show signals with context, never a bare verdict or "AI detected" label.
- Guard false positives: dictation, assistive tools, quoting their own earlier answer, and non-native writers must not be punished by default.
- The teacher decides. The system supplies the record.

## The two outputs

**Paper.** Built from the student's saved quotes, each tied to the dialogue turn it came from. The student expands in their own words under the same capture. The finished paper is checked against the record before it is certified.

**Test.** Teacher picks: questions generated from the objectives, teacher-written questions, or a mix. Closed-book. Same capture. Can be asked in the Socratic style (follow-ups probe the answer) for an oral-exam feel. Result: score per objective, plus the same evidence record.

## Where it runs

- One program (Go binary, SQLite file). A school runs it on one laptop or a server; students open it in a browser on the school network.
- AI provider is swappable. Default for schools: a local model through Ollama (private, free, offline). Option: bring-your-own cloud key.
- Honest limit: a small local model tutors worse than a large one, and judging grasp is harder than asking questions. Plan for a separate, stronger "judge" model setting, and test judging quality on real student answers before trusting it.
- Packaging: one download per OS, plus a short setup guide for non-technical teachers.

## Privacy and trust

- All student data stays on the school's machine by default. No telemetry.
- Teacher passcode on every professor route (v1 had none, so anyone could read every student's session).
- Retention and delete controls per course and per student.
- Student-facing page that states what is recorded, in plain words (typing and paste capture must never be hidden).
- Not legal advice: have FERPA and COPPA questions reviewed before putting it in front of schools, especially under-13 students.

## Data model changes

- `courses(id, title, syllabus_text, join_code, settings_json, created_at)`
- `objectives(id, course_id, position, text, rubric)`
- `school(name, admin passcode, policy{defaults, locked[], guidance})`, `teachers`, `auth_tokens`
- `sessions` gets `course_id`
- `concepts` becomes per-session copies of `objectives`, plus `evidence_quote`, `evidence_exchange`
- `capture_events(id, session_id, exchange_id, type, size, ts)` for paste, cadence, focus
- `tests`, `test_items`, `test_attempts`

## Known v1 problems to fix as we go

- Keyword grasp check (above).
- No professor auth.
- `GetSessionDetails` uses `SELECT *` into a fixed column list, which breaks if the schema changes.
- Backend pins `claude-sonnet-4-20250514`.
- Subject and topic typed by the student instead of coming from the course.
- Concept lists are a hard-coded map of 7 subjects.

## Build order

1. School and teacher layers, course and syllabus upload, objective extraction with teacher review, join code, passcode auth, grasp judge with verbatim evidence. **DONE and browser-tested 10-10-2026.** Not yet: PDF upload, test mode (the Test checkbox exists and says so).
2. Capture at the input box, teacher evidence view, paste-block setting. **DONE and browser-tested 10-10-2026.** Records typing, pasting, deletions and time away for the voice sample, every dialogue answer, the synthesis and the paper. Mostly pasted answers earn no objective credit. Pasting the student's own earlier words or the tutor's text is not held against them. Teachers can mark a student as using dictation or assistive tools: signals stay on record but stop lowering the score. The student sees a disclosure line on every writing screen. Authenticity score now weights capture facts first; the old keyword and style signals are capped so they cannot sink an honest student alone.
   - **Own words only (default paste policy, 10-11-2026).** Socratean saves everything the student writes and lets them paste it back freely, because those are their own original ideas. Pasting anything else is refused (teachers can instead block all pasting or allow and record it). Bulk inserts from autofill, extensions or AI keyboards are treated the same way.
   - **Consistency checks.** Every answer is compared with the student's interview, earlier answers, the tutor's words and the course material: long words that appeared nowhere before, a jump in word or sentence length, and a long pause followed by new vocabulary. These point to a possible second source (a phone, another person) and are hints for the teacher, not proof. Thresholds are guesses until a real-class pilot.
   - Limit, stated plainly: the browser reports these facts, so a determined student can forge or bypass them. They are evidence for a teacher to weigh, never proof.
   - Not done: a real-student pilot to tune the thresholds (fast-entry is 15 characters per second; mostly-pasted is 60%).
3. Test mode.
4. Swappable AI (Ollama, LM Studio or any OpenAI-compatible server, or Anthropic), separate grading model, admin AI tab with connection test and a 5-case grading check, no-model banner, pure-Go SQLite, one-command release build for five platforms, school setup guide. **DONE 10-11-2026.** Finding: llama3.2 3B scores 4 of 5 on the grading check (never over-credits, but will not credit a plain own-words explanation), so schools need a larger model. Not yet verified: any larger local model on the grading check, and a Windows or Linux run (cross-built only).
5. Pilot with one real class, then tune.

## Open questions for tUrUq

- Hosted for a school by us, or only self-run? (Self-run is the current plan.)
- Under-13 students in scope?
- Should the test also be allowed to lock out switching tabs, or is flagging enough?
- Name for the public release: "Socratean AI"?
