# Setting up Socratean AI at your school

Socratean runs on one computer at your school. Students and teachers open it in a web browser. Nothing is sent to us. Student writing goes only to the AI provider you choose (or nowhere, if you run the AI on your own computer).

## 1. Choose an AI (once)

In the **AI model** tab (step 3) pick one of three:

- **Claude (recommended).** Create an account with Anthropic, make an API key, paste it into the panel.
- **Another cloud provider.** OpenRouter, Together, Fireworks and others serve open models such as GLM-5.3-Flash (MIT license) at lower cost. Paste the provider's address, a model name, and your key.
- **On your own computer.** Install Ollama (ollama.com) and download a model. Student writing never leaves the building, but small models grade poorly. In our check, a 3 billion parameter model answered 4 of 5 grading examples correctly: it never wrongly credited a student, but it refused to credit clear explanations.

The key you paste is stored in this school's own database file and is never shown again. With any cloud choice, student writing is sent to that provider, so check its education or no-retention terms first. **Always run the grading check** (step 3) before trusting a model with real students.

## 2. Start Socratean

Double-click or run the program for your system from the `dist` folder (`socratean-windows-amd64.exe`, `socratean-darwin-arm64` for newer Macs, and so on).

The first time, it prints a **school admin passcode once**. Write it down. To choose your own, set `SOCRATEAN_ADMIN_PASSCODE` before starting.

Open `http://localhost:8080/teacher.html` and sign in with that passcode.

## 3. Set up the school

- **AI model tab:** pick your model, press *Test connection*, then *Run grading check*.
- **School tab:** set the defaults (how much understanding counts as done, whether pasting is allowed) and lock any rule teachers should not change. Add house rules for the tutor, such as tone and age-appropriateness.
- **Teachers tab:** add each teacher. Each gets their own passcode, shown once.

## 4. Teachers

Sign in at `/teacher.html`. Create a course: paste the syllabus, review the learning objectives Socratean drafts, and adjust the rules for that course. Give students the **course code**.

## 5. Students

Students open `http://<this computer's address>:8080` on the school network, enter their name and the course code, and begin. Find the address in your network settings. Keep this on your school network; do not expose it to the internet.

## Your data

Everything is stored in one file, `socratean.db`, next to the program. Copy it to back up. Delete it to erase everything. Students are told on every writing screen that their teacher can see how they write (typing, pasting, time away).

## Honest limits

- Typing and pasting records come from the student's browser. A determined student can bypass them. They are evidence for a teacher to weigh, not proof.
- Understanding is graded by an AI model. Small models make mistakes, so teachers should read the evidence quotes.
- This has not yet been piloted with a real class. Treat the thresholds as starting points.
- Check with your district about student-privacy rules (FERPA, COPPA) before use, especially with children under 13.
