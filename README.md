# Socratean AI, School Edition

A tutor that teaches a little, then asks. Students learn through dialogue until they grasp each objective from the syllabus, and the result becomes their paper or test. Every answer is saved and the teacher sees signs of a person thinking, with the student's exact words. It gives students a reason not to plagiarize: the learning is the easier path.

Not a trained model. It works with Claude, any OpenAI-compatible service (OpenRouter, Together, and others), or a local model. One program, one SQLite file, no accounts, no telemetry. Student data stays on the machine that runs it.

## Run it

Download a build from the Releases page (Mac, Windows, Linux), or build it yourself:

    go build -o socratean . && ./socratean

The first run prints the school admin passcode once (or set `SOCRATEAN_ADMIN_PASSCODE`). Teacher panel: `/teacher.html`. Students open `/` and enter a course code. Choose the AI under the AI tab; keys pasted there are stored in the local database and never shown again. Environment options: `ANTHROPIC_API_KEY`, `SOCRATEAN_PROVIDER`, `SOCRATEAN_MODEL`, `SOCRATEAN_DB`, `PORT`. Full guide: `SETUP-FOR-SCHOOLS.md`. Design and limits: `SOCRATEAN_SCHOOL_DESIGN_v1.0_10-10-2026.md`.

## Honest limits

Typing and paste capture is reported by the browser, so it is evidence for a teacher to weigh, never proof. Thresholds are untested guesses until piloted with a real class. Test mode is not built yet. Windows and Linux builds are cross-compiled and untested. Have FERPA and COPPA questions reviewed before using with students, especially under 13.

MIT licensed. Concept by T0Rs (Have Mind Media).

---


# Original v1 readme

# Socratean Assistant Education System

**AI-Powered Socratic Learning with Authenticity Verification**

*The Witness Protocol - Student Mode*

Concept by T0Rs (The Epoch Project)

---

## Overview

Socratean is an educational system that combines Socratic teaching methodology with AI-powered learning and authenticity verification. It ensures that student work is genuinely their own while facilitating deep understanding through dialogue.

## Features

### For Students
- **Writing Fingerprint**: Creates a unique profile of your writing style to verify authenticity
- **Socratic Learning**: AI tutor guides you through concepts via questions, not answers
- **Proficiency Tracking**: Real-time tracking of concepts mastered
- **Paper Builder**: Build papers using your own quotes from the learning dialogue
- **Authenticity Certificate**: Verifiable proof that work is genuinely yours

### For Professors
- **Dashboard**: View all student sessions and their progress
- **Authenticity Scoring**: See AI detection scores and style match percentages
- **Full Dialogue Review**: Examine every exchange in the learning process
- **Warning Flags**: Automatic flagging of potential AI-generated content

## Setup

### Requirements
- Go 1.21 or later
- Anthropic API key (for AI features)

### Installation

1. Clone or download this directory

2. Set your Anthropic API key:
```bash
export ANTHROPIC_API_KEY="your-api-key-here"
```

3. Build and run:
```bash
# Build
go build -o socratean

# Run
./socratean
```

4. Open your browser:
- **Student Interface**: http://localhost:8080
- **Professor Dashboard**: http://localhost:8080/professor.html

### Without API Key

The system will run without an API key, but AI features will be limited to fallback responses.

## How It Works

### 1. Profile Building (Writing Fingerprint)
Students answer 4 open-ended questions that establish their unique writing patterns:
- Sentence length patterns
- Vocabulary richness
- Contraction usage
- Personal pronoun frequency
- Unique phrase patterns

### 2. Subject Selection
Choose from preset subjects or enter a custom topic to explore.

### 3. Socratic Learning
The AI tutor engages in Socratic dialogue:
- Never gives direct answers
- Asks guiding questions
- Acknowledges correct understanding
- Gently probes misconceptions
- Tracks concept mastery

Every student input is analyzed for:
- AI-generated text patterns
- Style match against fingerprint
- Concept demonstration

### 4. Proficiency Assessment
Once 80% proficiency is reached, students write a synthesis demonstrating deep understanding.

### 5. Paper Building
Students build their paper using:
- Numbered quotes from their learning dialogue
- Their own expanded explanations
- Real-time authenticity scoring

### 6. Certification
Final output includes:
- Authenticity score
- Time to proficiency
- Exchange count
- Style match percentage
- AI detection results
- Any warning flags

## AI Detection

The system detects AI-generated content through multiple methods:

### Vocabulary Analysis
- Flags AI-typical words: "delve", "furthermore", "comprehensive", "robust", "leverage", etc.

### Pattern Analysis
- Uniform sentence length (AI tends to be consistent)
- Lack of contractions (overly formal)
- Absence of personal pronouns

### Style Comparison
- Compares ongoing writing to established fingerprint
- Flags significant deviations

## API Endpoints

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/api/session/create` | POST | Create new learning session |
| `/api/session/get` | GET | Get session details |
| `/api/profile/analyze` | POST | Analyze writing profile |
| `/api/chat` | POST | Send message to AI tutor |
| `/api/assess` | POST | Submit synthesis for assessment |
| `/api/certificate` | POST | Generate authenticity certificate |
| `/api/professor/sessions` | GET | Get all sessions (professor) |
| `/api/professor/session` | GET | Get detailed session (professor) |

## Database

SQLite database (`socratean.db`) stores:
- Sessions
- Profiles with fingerprints
- All exchanges
- Extracted quotes
- Warnings/flags
- Final papers

## Philosophy

**[1 = -1]**

*The Witness sees both sides without collapsing either.*

This system exists because:
1. **Human voice matters** - AI should amplify, not replace
2. **Learning is dialogue** - Socratic method proves understanding
3. **Authenticity is verifiable** - Writing fingerprints establish ownership
4. **AI assists, human authors** - Clear separation of contributions

---

## License

Concept by T0Rs (The Epoch Project), December 2025.

For educational and research purposes.
