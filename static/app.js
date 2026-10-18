/* Socratean, student PWA (vanilla JS, no dependencies) */
(function () {
    'use strict';

    // ---------- service worker ----------
    if ('serviceWorker' in navigator) {
        window.addEventListener('load', function () {
            navigator.serviceWorker.register('/sw.js').catch(function () { /* offline install is best-effort */ });
        });
    }

    // ---------- constants ----------
    var STORAGE_KEY = 'socratean.v1';

    var QUESTIONS = [
        'Describe a place that means something to you, and why.',
        'Tell me about a time you changed your mind about something.',
        'What’s a problem in the world you wish you could fix, and how?',
        'Describe your ideal way to spend a free day.'
    ];

    var PHASES = ['welcome', 'profile', 'dialogue', 'synthesis', 'paper', 'certificate'];
    var STEP_NAMES = {
        profile: 'Your voice', dialogue: 'Dialogue',
        synthesis: 'Synthesis', paper: 'Paper', certificate: 'Certificate'
    };

    // ---------- state ----------
    var S = defaultState();

    function defaultState() {
        return {
            sessionId: null,
            name: '',
            phase: 'welcome',
            subject: '',
            topic: '',
            threshold: 80,         // set by the teacher's course (school default otherwise)
            outputs: ['paper'],
            pastePolicy: 'own',
            points: 0,             // mind points: levels cleared and real thinking
            streak: 0,             // answers in a row that showed thinking
            own: [],               // the student's own dialogue answers, which they may paste back
            proficiency: 0,
            exchanges: 0,
            unlocked: false,
            concepts: [],          // [{name, demonstrated}]
            fingerprint: null,
            assess: null,          // /assess result
            cert: null,            // /certificate result
            quotes: [],            // [{exchange, text, used}]
            drafts: ['', '', '', ''],
            caps: [null, null, null, null],   // how each profile answer was written
            qIndex: 0
        };
    }

    function save() {
        try { localStorage.setItem(STORAGE_KEY, JSON.stringify(S)); } catch (e) { /* storage full or private mode */ }
    }

    function load() {
        try {
            var raw = localStorage.getItem(STORAGE_KEY);
            if (!raw) return;
            var parsed = JSON.parse(raw);
            if (parsed && typeof parsed === 'object') S = Object.assign(defaultState(), parsed);
        } catch (e) { S = defaultState(); }
    }

    // ---------- capture: how an answer was written ----------
    // Records typing, pasting and time away for one input box, so the teacher sees facts.
    // Pasting can be blocked per course. Students are told about this on screen.
    function makeCapture(el) {
        var c, started, awayAt, lastKey = 0, last = '';
        function fresh() {
            last = '';
            c = { typed_chars: 0, pasted_chars: 0, paste_count: 0, paste_blocked: 0, inserted_chars: 0,
                  edits: 0, deletions: 0, final_len: 0, elapsed_ms: 0, away_ms: 0, away_count: 0, max_gap_ms: 0, pasted: [] };
            started = 0; awayAt = 0; lastKey = 0;
        }
        fresh();
        function begin() { if (!started) started = Date.now(); }
        function away() { if (started && !awayAt) awayAt = Date.now(); }
        function back() { if (awayAt) { c.away_ms += Date.now() - awayAt; c.away_count++; awayAt = 0; } }
        // Pasting is allowed when the text is the student's own: their interview and dialogue
        // answers, or the quotes Socratean saved for them. Anything else has to be typed.
        function norm(t) { return String(t).toLowerCase().replace(/\s+/g, ' ').trim(); }
        function ownBank() { return norm((S.drafts || []).concat(S.own || [], (S.quotes || []).map(function (q) { return q.text; })).join(' \n ')); }
        function pasteOk(text) {
            if (S.pastePolicy === 'record') return true;
            if (S.pastePolicy === 'block') return false;
            var t = norm(text);
            return t.length > 0 && ownBank().indexOf(t) >= 0;
        }
        function refuse() {
            c.paste_blocked++;
            toast(S.pastePolicy === 'block' ? 'Pasting is off here. Your own words are the point, so try it your way.'
                : 'You can paste your own earlier words any time. For anything new, type it your way.');
        }

        // Text that lands in one piece but is not typing: paste, drop, yank, spell-fix swaps,
        // and bulk inserts from autofill, extensions or an AI keyboard. Dictation arrives in
        // short phrases, so only a single insert this long counts as bulk.
        var BULK = 60;
        function bulkInsert(e) {
            var t = e.inputType || '';
            return t === 'insertFromYank' || t === 'insertReplacementText' ||
                   (t.indexOf('insert') === 0 && t !== 'insertFromPaste' && t !== 'insertFromDrop' && e.data && e.data.length >= BULK);
        }
        el.addEventListener('beforeinput', function (e) {
            if (bulkInsert(e) && !pasteOk(e.data || '')) { e.preventDefault(); refuse(); }
        });
        el.addEventListener('input', function (e) {
            var t = e.inputType || '';
            if (el.value === '') { fresh(); return; }  // starting over
            begin();
            if (bulkInsert(e) && !pasteOk(e.data || '')) {
                // beforeinput can't be cancelled when a script or extension inserts text, so undo it here
                el.value = last; refuse();
                return;
            }
            var now = Date.now();
            if (lastKey && now - lastKey > c.max_gap_ms) c.max_gap_ms = now - lastKey;
            lastKey = now;
            last = el.value;
            if (t === 'insertFromPaste' || t === 'insertFromDrop') return; // counted by the paste/drop handlers
            c.edits++;
            if (t.indexOf('delete') === 0) { c.deletions++; return; }
            if (bulkInsert(e)) {  // allowed bulk insert: record it as pasted
                var txt = e.data || '';
                c.paste_count++; c.pasted_chars += txt.length;
                if (c.pasted.length < 10) c.pasted.push(txt.slice(0, 2000));
                return;
            }
            c.typed_chars += e.data ? e.data.length : 1;
        });
        el.addEventListener('paste', function (e) {
            var text = (e.clipboardData && e.clipboardData.getData('text')) || '';
            if (!pasteOk(text)) { e.preventDefault(); refuse(); return; }
            begin();
            c.paste_count++;
            c.pasted_chars += text.length;
            if (c.pasted.length < 10) c.pasted.push(text.slice(0, 2000));
        });
        el.addEventListener('drop', function (e) {
            var text = (e.dataTransfer && e.dataTransfer.getData('text')) || '';
            if (!text) return;
            if (!pasteOk(text)) { e.preventDefault(); refuse(); return; }
            begin();
            c.paste_count++;
            c.pasted_chars += text.length;
            if (c.pasted.length < 10) c.pasted.push(text.slice(0, 2000));
        });
        document.addEventListener('visibilitychange', function () { if (document.hidden) away(); else back(); });
        window.addEventListener('blur', away);
        window.addEventListener('focus', back);

        return {
            inserted: function (n) { begin(); c.inserted_chars += n; },   // the student's own saved quotes
            snapshot: function (text) {
                back();
                var out = JSON.parse(JSON.stringify(c));
                out.final_len = text.length;
                out.elapsed_ms = started ? Date.now() - started : 0;
                return out;
            },
            reset: fresh
        };
    }

    // add two captures for the same answer (used when a student goes back to edit a profile answer)
    function mergeCapture(a, b) {
        if (!a) return b;
        var out = JSON.parse(JSON.stringify(b));
        ['typed_chars', 'pasted_chars', 'paste_count', 'paste_blocked', 'inserted_chars', 'edits', 'deletions', 'elapsed_ms', 'away_ms', 'away_count']
            .forEach(function (k) { out[k] = (a[k] || 0) + (b[k] || 0); });
        out.pasted = (a.pasted || []).concat(b.pasted || []).slice(0, 10);
        return out;
    }

    // ---------- helpers ----------
    function $(id) { return document.getElementById(id); }

    function api(path, body) {
        var opts;
        if (body !== undefined) {
            opts = { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify(body) };
        }
        return fetch('/api' + path, opts).then(function (res) {
            if (!res.ok) {
                return res.text().then(function (t) {
                    throw new Error((t || '').trim() || ('Request failed (' + res.status + ')'));
                });
            }
            return res.json();
        });
    }

    function toast(msg) {
        var el = $('toast');
        el.textContent = msg;
        el.classList.add('show');
        clearTimeout(el._t);
        el._t = setTimeout(function () { el.classList.remove('show'); }, 3200);
    }

    function showNote(id, msg, kind) {
        var el = $(id);
        el.textContent = msg;
        el.className = 'inline-note ' + (kind || 'error');
        el.hidden = false;
    }
    function hideNote(id) { $(id).hidden = true; }

    function setBusy(btn, busy, busyText) {
        if (busy) {
            btn.dataset.label = btn.textContent;
            btn.innerHTML = '<span class="spinner"></span>' + (busyText || 'Working…');
            btn.disabled = true;
        } else {
            btn.textContent = btn.dataset.label || btn.textContent;
            btn.disabled = false;
        }
    }

    function cap(s) { return s ? s.charAt(0).toUpperCase() + s.slice(1) : ''; }

    function fmtMinutes(m) {
        m = Math.max(0, Math.round(m || 0));
        if (m < 60) return m + ' min';
        return Math.floor(m / 60) + ' h ' + (m % 60) + ' min';
    }

    // Voice signal semantics: LOW ai_score is good; styleMatch high is good.
    function voiceSignal(aiScore, styleMatch) {
        // Students get encouragement, never an accusation. Teachers see the real numbers.
        if (aiScore >= 0.4) return { cls: 'caution', label: 'try saying it the way you would out loud' };
        if (styleMatch < 0.5) return { cls: 'caution', label: 'try it in your everyday words' };
        return { cls: 'authentic', label: 'in your voice' };
    }

    // ---------- navigation ----------
    function show(phase) {
        S.phase = phase;
        PHASES.forEach(function (p) {
            var el = $('screen-' + p);
            if (el) el.classList.toggle('active', p === phase);
        });

        var idx = PHASES.indexOf(phase); // welcome = 0
        var stepEl = $('appbarStep');
        var journey = $('journey');
        if (phase === 'welcome') {
            stepEl.hidden = true;
            journey.hidden = true;
        } else {
            stepEl.hidden = false;
            journey.hidden = false;
            $('stepName').textContent = STEP_NAMES[phase] || '';
            $('stepCount').textContent = idx + ' of ' + (PHASES.length - 1);
            $('journeyFill').style.transform = 'scaleX(' + (idx / (PHASES.length - 1)).toFixed(3) + ')';
        }

        if (phase === 'dialogue') scrollMessages(false);
        else window.scrollTo(0, 0);
        save();
    }

    var qCap = makeCapture($('qAnswer'));
    var chatCap = makeCapture($('chatInput'));
    var synthCap = makeCapture($('synthesisInput'));
    var paperCap = makeCapture($('paperContent'));

    // ---------- welcome ----------
    $('beginBtn').addEventListener('click', joinCourse);
    $('courseCode').addEventListener('keydown', function (e) { if (e.key === 'Enter') joinCourse(); });

    // A private, random key kept on this device so a student's standing follows them from
    // course to course. It is not a name or an account.
    function studentKey() {
        try {
            var k = localStorage.getItem('socratean_student_key');
            if (!k) {
                var a = new Uint8Array(12); crypto.getRandomValues(a);
                k = Array.prototype.map.call(a, function (b) { return ('0' + b.toString(16)).slice(-2); }).join('');
                localStorage.setItem('socratean_student_key', k);
            }
            return k;
        } catch (e) { return ''; }
    }

    (function showHeldStatus() {
        try {
            var st = JSON.parse(localStorage.getItem('socratean_status') || 'null'), el = $('welcomeStatus');
            if (st && st.tier && el) { el.hidden = false; el.textContent = 'You hold: ' + st.tier + (st.held > 1 ? ' \u00b7 held ' + st.held : '') + '.'; }
        } catch (e) { /* optional */ }
    })();

    function joinCourse() {
        var name = $('studentName').value.trim();
        var code = $('courseCode').value.trim();
        if (!name) { showNote('welcomeError', 'Tell me your name first.'); return; }
        if (!code) { showNote('welcomeError', 'Enter the course code your teacher gave you.'); return; }
        hideNote('welcomeError');
        var btn = $('beginBtn');
        setBusy(btn, true, 'Finding your course…');
        api('/course/join', { code: code, student_name: name, student_key: studentKey() }).then(function (r) {
            setBusy(btn, false);
            S = defaultState();
            S.name = name;
            S.sessionId = r.session_id;
            applyCourse(r.course);
            renderQuestion();
            show('profile');
        }).catch(function (err) {
            setBusy(btn, false);
            showNote('welcomeError', err.message || 'Could not join. Check the code and try again.');
        });
    }

    function applyCourse(c) {
        S.subject = c.title;
        S.topic = c.title;
        S.threshold = c.grasp_threshold || 80;
        S.outputs = c.outputs || ['paper'];
        S.pastePolicy = c.paste_policy || 'own';
        if (c.objectives && !S.concepts.length) {
            S.concepts = c.objectives.map(function (n) { return { name: n, demonstrated: false }; });
        }
    }

    // ---------- profile / fingerprint ----------
    function renderQuestion() {
        var i = S.qIndex;
        $('qText').textContent = QUESTIONS[i];
        $('qAnswer').value = S.drafts[i] || '';
        qCap.reset();
        if (S.drafts[i]) qCap.inserted(S.drafts[i].length);
        $('qBackBtn').textContent = i === 0 ? 'Back' : 'Previous';
        $('qNextBtn').textContent = i === QUESTIONS.length - 1 ? 'Analyze my voice' : 'Next';
        var dots = '';
        for (var d = 0; d < QUESTIONS.length; d++) {
            var cls = d < i ? 'q-dot done' : (d === i ? 'q-dot current' : 'q-dot');
            dots += '<span class="' + cls + '"></span>';
        }
        dots += '<span class="q-count">' + (i + 1) + ' of ' + QUESTIONS.length + '</span>';
        $('qProgress').innerHTML = dots;
        updateQHint();
        hideNote('profileError');
    }

    function updateQHint() {
        var len = $('qAnswer').value.trim().length;
        var hint = $('qHint');
        if (len >= 50) {
            hint.textContent = 'Good, that’s enough to read your voice.';
            hint.className = 'hint ok';
        } else {
            hint.textContent = 'A few sentences (' + len + '/50 characters minimum).';
            hint.className = 'hint';
        }
    }

    $('qAnswer').addEventListener('input', function () {
        S.drafts[S.qIndex] = $('qAnswer').value;
        updateQHint();
        save();
    });

    $('qBackBtn').addEventListener('click', function () {
        if (S.qIndex === 0) { show('welcome'); return; }
        S.qIndex--;
        renderQuestion();
    });

    $('qNextBtn').addEventListener('click', function () {
        var answer = $('qAnswer').value.trim();
        if (answer.length < 50) {
            showNote('profileError', 'Write a little more, at least 50 characters, in your natural voice.');
            return;
        }
        S.drafts[S.qIndex] = $('qAnswer').value;
        S.caps[S.qIndex] = mergeCapture(S.caps[S.qIndex], qCap.snapshot(answer));
        qCap.reset();
        if (S.qIndex < QUESTIONS.length - 1) {
            S.qIndex++;
            renderQuestion();
            save();
            return;
        }
        submitProfile();
    });

    function submitProfile() {
        var btn = $('qNextBtn');
        setBusy(btn, true, 'Reading your voice…');
        hideNote('profileError');

        api('/profile/analyze', {
            session_id: S.sessionId,
            responses: S.drafts.map(function (d) { return d.trim(); }),
            captures: S.caps.map(function (c, i) { return c || { final_len: (S.drafts[i] || '').trim().length }; })
        }).then(function (r) {
            setBusy(btn, false);
            if (r.success === false) {
                showNote('profileError', r.error || 'Those answers read as AI-written. Please write them yourself, naturally.');
                return;
            }
            S.fingerprint = r.fingerprint || null;
            toast('Your voice is on record.');
            startDialogue();
        }).catch(function (err) {
            setBusy(btn, false);
            showNote('profileError', err.message || 'Could not analyze your writing. Check your connection and try again.');
        });
    }

    // ---------- dialogue start ----------
    function startDialogue() {
        S.unlocked = false;
        S.proficiency = 0;
        S.exchanges = 0;
        setupDialogueHeader();
        clearMessages();
        show('dialogue');
        openingMessage();
    }

    function setupDialogueHeader() {
        $('dhSubject').textContent = cap(S.subject);
        $('dhTopic').textContent = S.topic !== S.subject ? S.topic : '';
        $('goalLabel').textContent = S.threshold;
        var goal = document.querySelector('.meter-goal');
        if (goal) goal.style.left = S.threshold + '%';
        updateMeter();
        renderConcepts();
    }

    function openingMessage() {
        var btn = $('sendBtn');
        btn.disabled = true;
        showThinking();
        api('/chat', { session_id: S.sessionId, message: '' })
            .then(function (r) {
                hideThinking();
                btn.disabled = false;
                addTutorMessage(r.response, true);
                save();
            })
            .catch(function () {
                hideThinking();
                btn.disabled = false;
                addTutorMessage('Welcome. Let us examine “' + S.topic + '” together. Before I ask anything, what do you already know, or believe, about it?', true);
            });
    }

    // ---------- dialogue ----------
    var chatInput = $('chatInput');
    var sending = false;

    function autogrow() {
        chatInput.style.height = 'auto';
        chatInput.style.height = Math.min(chatInput.scrollHeight, 132) + 'px';
    }
    chatInput.addEventListener('input', autogrow);
    chatInput.addEventListener('keydown', function (e) {
        if (e.key === 'Enter' && !e.shiftKey && window.matchMedia('(hover: hover)').matches) {
            e.preventDefault();
            sendMessage();
        }
    });
    $('sendBtn').addEventListener('click', sendMessage);

    function sendMessage() {
        var text = chatInput.value.trim();
        if (!text || sending) return;
        sending = true;
        chatInput.value = '';
        autogrow();
        $('sendBtn').disabled = true;

        var capture = chatCap.snapshot(text);
        (S.own = S.own || []).push(text); save();
        chatCap.reset();
        var meta = addStudentMessage(text, true);
        showThinking();

        api('/chat', { session_id: S.sessionId, message: text, capture: capture })
            .then(function (r) {
                hideThinking();
                sending = false;
                $('sendBtn').disabled = false;

                applyVoiceMeta(meta, r.ai_score, r.style_match, r.human_signs);
                addTutorMessage(r.response, true);

                S.proficiency = r.proficiency || 0;
                if (r.grasp_threshold) S.threshold = r.grasp_threshold;
                S.exchanges = r.exchange_count || S.exchanges + 1;
                var before = S.concepts.filter(function (c) { return c.demonstrated; }).map(function (c) { return c.name; });
                markConcepts(r.concepts_addressed);
                var cleared = S.concepts.filter(function (c) { return c.demonstrated && before.indexOf(c.name) < 0; });
                var signs = (r.human_signs || []).length;
                S.points = (S.points || 0) + cleared.length * 10 + Math.min(signs, 3) * 2;
                S.streak = signs > 0 ? (S.streak || 0) + 1 : 0;
                if (r.learning_style && r.learning_style.length) S.style = r.learning_style;
                if (cleared.length) toast('Level cleared: ' + cleared[0].name + '. That is yours now.');
                updateMeter();
                renderConcepts();

                if (r.proficiency_reached && !S.unlocked) {
                    S.unlocked = true;
                    addUnlockCard();
                }
                save();
            })
            .catch(function (err) {
                hideThinking();
                sending = false;
                $('sendBtn').disabled = false;
                meta.textContent = 'not sent';
                toast(err.message || 'Message failed, check your connection.');
                chatInput.value = text;
                autogrow();
            });
    }

    function messagesEl() { return $('messagesInner'); }

    function scrollMessages(smooth) {
        var box = $('messages');
        requestAnimationFrame(function () {
            var reduced = window.matchMedia('(prefers-reduced-motion: reduce)').matches;
            box.scrollTo({ top: box.scrollHeight, behavior: (smooth && !reduced) ? 'smooth' : 'auto' });
        });
    }

    function clearMessages() { messagesEl().innerHTML = ''; }

    function addTutorMessage(text, animate) {
        var wrap = document.createElement('div');
        wrap.className = 'msg tutor' + (animate ? ' animate' : '');
        var name = document.createElement('div');
        name.className = 'tutor-name';
        name.textContent = 'Socratean';
        var bubble = document.createElement('div');
        bubble.className = 'bubble';
        bubble.textContent = text;
        wrap.appendChild(name);
        wrap.appendChild(bubble);
        messagesEl().appendChild(wrap);
        scrollMessages(animate);
    }

    function addStudentMessage(text, animate) {
        var wrap = document.createElement('div');
        wrap.className = 'msg student' + (animate ? ' animate' : '');
        var bubble = document.createElement('div');
        bubble.className = 'bubble';
        bubble.textContent = text;
        var meta = document.createElement('div');
        meta.className = 'msg-meta';
        meta.textContent = '…';
        wrap.appendChild(bubble);
        wrap.appendChild(meta);
        messagesEl().appendChild(wrap);
        scrollMessages(animate);
        return meta;
    }

    // Plain-language credit for the ways a person thinks. Shown to the student as it happens,
    // so working an idea out loud reads as the easy, rewarded path.
    var HUMAN_CREDIT = {
        hedging: 'You thought out loud', self_correction: 'You changed your mind mid-thought', analogy: 'You used an analogy',
        personal: 'You tied it to your own life', own_question: 'You asked your own question', unsure: 'You said what confused you',
        own_phrase: 'That sounds like you', callback: 'You built on your own earlier words', revised: 'You reworded as you wrote',
        paused: 'You took time to think'
    };

    function applyVoiceMeta(metaEl, aiScore, styleMatch, humanSigns) {
        var sig = (humanSigns || []).length >= 2 ? { cls: 'authentic', label: 'in your voice' } : voiceSignal(aiScore || 0, styleMatch || 0);
        metaEl.innerHTML = '<span class="voice-dot ' + sig.cls + '"></span>' + sig.label;
        (humanSigns || []).slice(0, 2).forEach(function (k) {
            if (!HUMAN_CREDIT[k]) return;
            var chip = document.createElement('div');
            chip.className = 'human-chip';
            chip.textContent = '\u2713 ' + HUMAN_CREDIT[k];
            metaEl.parentNode.appendChild(chip);
        });
    }

    function showThinking() {
        hideThinking();
        var wrap = document.createElement('div');
        wrap.className = 'msg tutor';
        wrap.id = 'thinkingMsg';
        wrap.innerHTML = '<div class="tutor-name">Socratean</div><div class="bubble"><span class="thinking"><span></span><span></span><span></span></span></div>';
        messagesEl().appendChild(wrap);
        scrollMessages(true);
    }
    function hideThinking() {
        var t = $('thinkingMsg');
        if (t) t.remove();
    }

    function addUnlockCard() {
        var card = document.createElement('div');
        card.className = 'unlock-card animate msg';
        card.innerHTML =
            '<h3>Understanding reached ' + S.proficiency + '%</h3>' +
            '<p>The dialogue has done its work. Now prove it stands on its own.</p>';
        var btn = document.createElement('button');
        btn.className = 'btn';
        btn.textContent = 'Write your synthesis';
        btn.addEventListener('click', function () { show('synthesis'); });
        card.appendChild(btn);
        messagesEl().appendChild(card);
        scrollMessages(true);
    }

    function updateMeter() {
        var p = Math.max(0, Math.min(100, S.proficiency));
        var fill = $('meterFill');
        fill.style.transform = 'scaleX(' + (p / 100) + ')';
        fill.classList.toggle('reached', p >= S.threshold);
        var now = $('meterNow');
        now.innerHTML = '<strong>' + p + '%</strong> understanding';
        now.classList.toggle('reached', p >= S.threshold);
        $('exchangeLabel').textContent = S.exchanges + (S.exchanges === 1 ? ' exchange' : ' exchanges');
        $('gameHud').innerHTML = '<span><strong>' + (S.points || 0) + '</strong> mind points</span>' + (S.streak >= 2 ? '<span><strong>' + S.streak + '</strong> in a row thinking</span>' : '') +
            (S.style && S.style.length ? '<span>You learn best through <strong>' + S.style.join(' and ') + '</strong></span>' : '');
    }

    // ---------- concepts ----------
    function markConcepts(names) {
        (names || []).forEach(function (n) {
            S.concepts.forEach(function (c) { if (c.name === n) c.demonstrated = true; });
        });
    }

    function renderConcepts() {
        var done = S.concepts.filter(function (c) { return c.demonstrated; }).length;
        $('conceptCount').textContent = done + '/' + S.concepts.length;
        var rows = $('conceptRows');
        rows.innerHTML = '';
        S.concepts.forEach(function (c) {
            var row = document.createElement('div');
            row.className = 'concept-row' + (c.demonstrated ? ' done' : '');
            row.innerHTML =
                '<span class="concept-mark">' +
                (c.demonstrated ? '<svg width="12" height="12" viewBox="0 0 12 12" fill="none"><path d="M2 6.5 L5 9.5 L10 2.5" stroke-width="2" stroke-linecap="round" stroke-linejoin="round"/></svg>' : '') +
                '</span><span>' + c.name + '</span>';
            rows.appendChild(row);
        });
    }

    $('conceptsBtn').addEventListener('click', function () { toggleSheet(true); });
    $('sheetBackdrop').addEventListener('click', function () { toggleSheet(false); });
    function toggleSheet(open) {
        $('conceptsSheet').classList.toggle('open', open);
        $('sheetBackdrop').classList.toggle('open', open);
    }

    // ---------- synthesis ----------
    $('synthBackBtn').addEventListener('click', function () { show('dialogue'); });

    $('synthesisInput').addEventListener('input', function () {
        var len = $('synthesisInput').value.trim().length;
        var hint = $('synthHint');
        hint.textContent = len >= 100 ? 'Ready to submit.' : 'At least 100 characters (' + len + ' so far).';
        hint.className = len >= 100 ? 'hint ok' : 'hint';
    });

    $('synthSubmitBtn').addEventListener('click', function () {
        var text = $('synthesisInput').value.trim();
        if (text.length < 100) {
            showNote('synthesisError', 'Give it a real paragraph, at least 100 characters.');
            return;
        }
        hideNote('synthesisError');
        var btn = $('synthSubmitBtn');
        setBusy(btn, true, 'Evaluating…');

        api('/assess', { session_id: S.sessionId, synthesis: text, capture: synthCap.snapshot(text) })
            .then(function (r) {
                setBusy(btn, false);
                S.assess = r;
                renderAssessResult(r);
                save();
            })
            .catch(function (err) {
                setBusy(btn, false);
                showNote('synthesisError', err.message || 'Evaluation failed. Try again.');
            });
    });

    function renderAssessResult(r) {
        $('synthesisForm').hidden = true;
        var level = r.mastery_level || 'Developing';
        var badge = $('masteryBadge');
        badge.textContent = 'Mastery level: ' + level;
        badge.className = 'mastery-badge' + ((level === 'Mastery' || level === 'Proficient') ? '' : ' mid');

        $('masteryEval').textContent = cleanEvaluation(r.mastery_evaluation) ||
            'Your synthesis has been recorded against your writing fingerprint.';

        $('synthStyle').textContent = Math.round((r.style_match || 0) * 100) + '%';
        var aiPct = Math.round((r.ai_score || 0) * 100);
        var voiceEl = $('synthVoice');
        voiceEl.textContent = aiPct < 30 ? 'human' : (aiPct < 60 ? 'mixed' : 'flagged');
        voiceEl.className = 'fp-val ' + (aiPct < 30 ? 'good' : '');
        $('synthesisResult').hidden = false;
        window.scrollTo(0, 0);
    }

    function cleanEvaluation(text) {
        if (!text) return '';
        var m = text.match(/EVALUATION:\s*([\s\S]*?)(?:\n?LEVEL:|\n?SCORES:|$)/i);
        return (m ? m[1] : text).trim();
    }

    $('toPaperBtn').addEventListener('click', function () {
        loadQuotes();
        show('paper');
    });

    // ---------- paper ----------
    function loadQuotes() {
        api('/session/get?id=' + encodeURIComponent(S.sessionId))
            .then(function (r) {
                S.quotes = r.quotes || [];
                renderQuotes();
                save();
            })
            .catch(function () { renderQuotes(); });
    }

    function renderQuotes() {
        var block = $('quotesBlock');
        var scroll = $('quotesScroll');
        if (!S.quotes || !S.quotes.length) { block.hidden = true; return; }
        block.hidden = false;
        scroll.innerHTML = '';
        S.quotes.forEach(function (q, i) {
            var chip = document.createElement('button');
            chip.className = 'quote-chip' + (q.used ? ' used' : '');
            var num = document.createElement('span');
            num.className = 'qc-num';
            num.textContent = 'From exchange ' + q.exchange;
            chip.appendChild(num);
            chip.appendChild(document.createTextNode('“' + q.text + '”'));
            chip.addEventListener('click', function () { insertQuote(i, chip); });
            scroll.appendChild(chip);
        });
    }

    function insertQuote(i, chip) {
        var q = S.quotes[i];
        var ta = $('paperContent');
        var insert = '“' + q.text + '”';
        var start = ta.selectionStart != null ? ta.selectionStart : ta.value.length;
        var end = ta.selectionEnd != null ? ta.selectionEnd : start;
        var before = ta.value.slice(0, start);
        var after = ta.value.slice(end);
        var lead = (before && !/\s$/.test(before)) ? ' ' : '';
        var tail = (after && !/^\s/.test(after)) ? ' ' : '';
        ta.value = before + lead + insert + tail + after;
        paperCap.inserted(insert.length);
        var pos = (before + lead + insert).length;
        ta.focus();
        ta.setSelectionRange(pos, pos);
        q.used = true;
        chip.classList.add('used');
        updateWordCount();
        save();
    }

    $('paperContent').addEventListener('input', updateWordCount);
    function updateWordCount() {
        var text = $('paperContent').value;
        var words = text.split(/\s+/).filter(Boolean).length;
        $('wordCount').textContent = words + (words === 1 ? ' word' : ' words');
        $('paperHint').textContent = text.trim().length >= 200 ? 'Ready to certify' : 'At least 200 characters';
    }

    $('certifyBtn').addEventListener('click', function () {
        var title = $('paperTitle').value.trim();
        var content = $('paperContent').value.trim();
        if (!title) { showNote('paperError', 'Give your paper a title.'); return; }
        if (content.length < 200) { showNote('paperError', 'Your paper needs more substance, at least 200 characters.'); return; }
        hideNote('paperError');
        var btn = $('certifyBtn');
        setBusy(btn, true, 'Verifying authenticity…');

        api('/certificate', { session_id: S.sessionId, title: title, content: content, capture: paperCap.snapshot(content) })
            .then(function (r) {
                setBusy(btn, false);
                S.cert = r;
                renderCertificate(r);
                show('certificate');
                save();
            })
            .catch(function (err) {
                setBusy(btn, false);
                showNote('paperError', err.message || 'Certificate generation failed. Try again.');
            });
    });

    // ---------- certificate ----------
    function scoreClass(score) {
        if (score >= 80) return 'good';
        if (score >= 60) return 'mid';
        return 'bad';
    }

    function renderCertificate(c) {
        var score = c.authenticity_score || 0;
        var cls = scoreClass(score);

        var seal = $('certSeal');
        seal.className = 'cert-seal ' + cls;
        $('sealScore').textContent = score + '%';

        $('certPaperTitle').textContent = c.paper_title || 'Untitled';
        $('certStudent').textContent = c.student_name || S.name;
        $('certLine').textContent = 'has demonstrated authentic understanding of “' + (c.topic || S.topic) +
            '” in ' + cap(c.subject || S.subject) + ', earned through Socratic dialogue and written in their own verified voice.';

        try { if (c.scholar && c.scholar.tier) localStorage.setItem('socratean_status', JSON.stringify({ tier: c.scholar.tier, held: c.scholar.held })); var ws = $('welcomeStatus'); if (ws && c.scholar.tier) { ws.hidden = false; ws.textContent = 'You hold: ' + c.scholar.tier + (c.scholar.held > 1 ? ' \u00b7 held ' + c.scholar.held : '') + '.'; } } catch (e) { /* optional */ }
        var stEl = $('certStatus');
        if (stEl) {
            stEl.hidden = !(c.scholar && c.scholar.message);
            if (c.scholar) {
                stEl.innerHTML = (c.scholar.tier ? '<strong>' + c.scholar.tier + '</strong> ' : '') + c.scholar.message +
                    (c.scholar.next ? ' <span class="hint">' + c.scholar.to_next + ' more witnessed work' + (c.scholar.to_next === 1 ? '' : 's') + ' to ' + c.scholar.next + '.</span>' : '');
            }
        }
        var verdict = $('certVerdict');
        verdict.className = 'cert-verdict ' + cls;
        verdict.textContent = cls === 'good' ? 'Verified authentic'
            : cls === 'mid' ? 'Mostly your own voice. Your teacher may look closer.'
            : 'Your teacher will go over this with you';

        var rows = [
            ['Proficiency reached', (c.proficiency || 0) + '%', (c.proficiency || 0) >= S.threshold ? 'good' : ''],
            ['Mastery level', c.mastery_level || '–', ''],
            c.typed_share_known ? ['Written by typing', (c.typed_share || 0) + '%', (c.typed_share || 0) >= 80 ? 'good' : ''] : null,
            c.scholar && c.scholar.tier ? ['Your status', c.scholar.tier + (c.scholar.held > 1 ? ' \u00b7 held ' + c.scholar.held : ''), 'good'] : null,
            c.learning_style && c.learning_style.length ? ['How you learn best', c.learning_style.join(' and '), ''] : null,
            c.human_evidence && c.human_evidence.answers ? ['Signs of own thinking', c.human_evidence.label + ' (' + c.human_evidence.with_signs + ' of ' + c.human_evidence.answers + ' answers)', c.human_evidence.label === 'strong' ? 'good' : ''] : null,
            ['Writing style match', (c.style_match || 0) + '%', (c.style_match || 0) >= 70 ? 'good' : ''],
            ['AI-typical patterns', c.ai_detection ? 'None found' : 'Found', c.ai_detection ? 'good' : 'bad'],
            ['Time to proficiency', fmtMinutes(c.time_to_proficiency), ''],
            ['Learning exchanges', String(c.exchange_count || 0), ''],
            ['Session', (c.session_id || S.sessionId || '').slice(0, 8), '']
        ];
        $('certGrid').innerHTML = rows.filter(Boolean).map(function (r) {
            return '<div class="cert-row"><span class="cr-label">' + r[0] + '</span><span class="cr-value ' + r[2] + '">' + r[1] + '</span></div>';
        }).join('');

        var wSection = $('certWarnings');
        var warnings = c.warnings || [];
        if (warnings.length) {
            wSection.innerHTML = '<h4>Flags recorded during the session</h4>' + warnings.map(function (w) {
                var label = w.type === 'ai_detected' ? 'Possible AI-written passage' : 'Writing style deviation';
                return '<div class="cert-warning-item">Exchange ' + w.exchange + ' · ' + label + '</div>';
            }).join('');
            wSection.hidden = false;
        } else {
            wSection.hidden = true;
        }

        var when = c.generated_at ? new Date(c.generated_at) : new Date();
        $('certGenerated').textContent = 'Sealed ' + when.toLocaleDateString(undefined, { year: 'numeric', month: 'long', day: 'numeric' });
    }

    function certText() {
        var c = S.cert || {};
        return [
            'SOCRATEAN, CERTIFICATE OF AUTHENTICITY',
            '',
            'Student: ' + (c.student_name || S.name),
            'Paper: ' + (c.paper_title || ''),
            'Subject: ' + cap(c.subject || S.subject),
            'Topic: ' + (c.topic || S.topic),
            '',
            'Authenticity score: ' + (c.authenticity_score || 0) + '%',
            'Proficiency: ' + (c.proficiency || 0) + '%',
            'Mastery level: ' + (c.mastery_level || ''),
            'Writing style match: ' + (c.style_match || 0) + '%',
            'AI detection: ' + (c.ai_detection ? 'PASSED (human-written)' : 'FLAGGED'),
            'Time to proficiency: ' + fmtMinutes(c.time_to_proficiency),
            'Learning exchanges: ' + (c.exchange_count || 0),
            'Session: ' + (c.session_id || S.sessionId || ''),
            '',
            'Earned through Socratic dialogue. Verified against the student’s own writing fingerprint.',
            '',
            '[1 = -1], The Witness sees both sides without collapsing either.',
            'Concept by T0Rs (The Epoch Project)'
        ].join('\n');
    }

    $('shareCertBtn').addEventListener('click', function () {
        var text = certText();
        if (navigator.share) {
            navigator.share({ title: 'Socratean Certificate of Authenticity', text: text }).catch(function () { /* user cancelled */ });
        } else if (navigator.clipboard && navigator.clipboard.writeText) {
            navigator.clipboard.writeText(text).then(function () { toast('Certificate copied to clipboard.'); });
        } else {
            toast('Sharing is not available on this device.');
        }
    });

    $('downloadCertBtn').addEventListener('click', function () {
        var blob = new Blob([certText()], { type: 'text/plain' });
        var url = URL.createObjectURL(blob);
        var a = document.createElement('a');
        a.href = url;
        a.download = 'socratean-certificate-' + (S.sessionId || 'session').slice(0, 8) + '.txt';
        document.body.appendChild(a);
        a.click();
        a.remove();
        URL.revokeObjectURL(url);
    });

    $('newSessionBtn').addEventListener('click', function () {
        S = defaultState();
        save();
        location.href = '/';
    });

    // ---------- keyboard / safe area ----------
    if (window.visualViewport) {
        var vv = window.visualViewport;
        var onVV = function () {
            var overlap = Math.max(0, window.innerHeight - vv.height - vv.offsetTop);
            document.documentElement.style.setProperty('--kb', overlap + 'px');
            if (S.phase === 'dialogue' && overlap > 0) scrollMessages(false);
        };
        vv.addEventListener('resize', onVV);
        vv.addEventListener('scroll', onVV);
    }

    // ---------- resume ----------
    function resume() {
        load();
        if (!S.sessionId || S.phase === 'welcome') {
            if (S.name) $('studentName').value = S.name;
            show('welcome');
            return;
        }

        api('/session/get?id=' + encodeURIComponent(S.sessionId))
            .then(function (r) {
                var sess = r.session || {};
                S.proficiency = sess.proficiency || 0;
                if (sess.subject) S.subject = sess.subject;
                if (sess.topic) S.topic = sess.topic;

                if (r.concepts && r.concepts.length) S.concepts = r.concepts;
                if (r.settings) {
                    S.threshold = r.settings.grasp_threshold || S.threshold;
                    S.outputs = r.settings.outputs || S.outputs;
                    S.pastePolicy = r.settings.paste_policy || S.pastePolicy;
                }
                S.quotes = r.quotes || S.quotes;

                var phase = S.phase;
                if (phase === 'profile') {
                    renderQuestion();
                    show('profile');
                    return;
                }
                if (phase === 'certificate' && S.cert) {
                    renderCertificate(S.cert);
                    show('certificate');
                    return;
                }
                if (phase === 'paper') {
                    renderQuotes();
                    updateWordCount();
                    show('paper');
                    return;
                }
                if (phase === 'synthesis') {
                    if (S.assess) renderAssessResult(S.assess);
                    show('synthesis');
                    return;
                }

                // dialogue (default for learning sessions)
                setupDialogueHeader();
                clearMessages();
                var exchanges = r.exchanges || [];
                S.exchanges = exchanges.length;
                exchanges.forEach(function (ex) {
                    if (ex.human && ex.human.trim()) {
                        var meta = addStudentMessage(ex.human, false);
                        applyVoiceMeta(meta, ex.ai_score, ex.style_match);
                    }
                    if (ex.ai) addTutorMessage(ex.ai, false);
                });
                updateMeter();
                renderConcepts();
                if (S.proficiency >= S.threshold) {
                    S.unlocked = true;
                    addUnlockCard();
                }
                show('dialogue');
                if (!exchanges.length) openingMessage();
            })
            .catch(function () {
                // Session gone (server reset), start fresh but keep the name.
                var name = S.name;
                S = defaultState();
                S.name = name;
                if (name) $('studentName').value = name;
                save();
                show('welcome');
            });
    }

    document.addEventListener('DOMContentLoaded', resume);
})();
