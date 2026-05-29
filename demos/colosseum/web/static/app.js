/* =============================================================================
   The Colosseum — replay viewer logic (vanilla JS, no dependencies)

   Talks to the same-origin JSON API:
     GET /api/matches            -> { matches: [MatchSummary] }   (newest first)
     GET /api/matches/{id}       -> { match: Match, analysis: Analysis }
     GET /api/leaderboard        -> { standings: [Row] }          (sorted, best first)

   Centerpiece: the "Reveal private reasoning" toggle unmasks each player's
   hidden chain of thought — night actions, the Seer's visions, and the private
   reasoning behind every statement and vote, plus the players' secret roles.

   All model-generated text is inserted via textContent (never innerHTML), so the
   UI is safe against injection from match messages and reasoning.
   ============================================================================= */
"use strict";

/* ---- Provider color map (kept in sync with CSS --p-* variables) ---------- */
const PROVIDER_COLORS = {
  claude: "#d97757",
  gpt:    "#34c98a",
  openai: "#34c98a",
  gemini: "#5b8def",
  google: "#5b8def",
  grok:   "#8b93a7",
  xai:    "#8b93a7",
};
const DEFAULT_COLOR = "#9aa0c0";

function providerColor(provider) {
  if (!provider) return DEFAULT_COLOR;
  return PROVIDER_COLORS[provider.toLowerCase()] || DEFAULT_COLOR;
}

/* ---- Tiny DOM helpers ---------------------------------------------------- */
const $ = (sel, root = document) => root.querySelector(sel);

/**
 * el(tag, attrs, children) builds an element safely.
 * - attrs: { class, title, dataset:{}, style:{}, text } and any other attribute.
 *   `text` sets textContent (escaped); never use innerHTML for model output.
 * - children: a node, string, or array thereof.
 */
function el(tag, attrs = {}, children = []) {
  const node = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (v == null) continue;
    if (k === "class") node.className = v;
    else if (k === "text") node.textContent = v;
    else if (k === "dataset") Object.assign(node.dataset, v);
    else if (k === "style") Object.assign(node.style, v);
    else node.setAttribute(k, v);
  }
  const kids = Array.isArray(children) ? children : [children];
  for (const c of kids) {
    if (c == null) continue;
    node.appendChild(typeof c === "string" ? document.createTextNode(c) : c);
  }
  return node;
}

function clear(node) { while (node.firstChild) node.removeChild(node.firstChild); }

async function fetchJSON(url) {
  const res = await fetch(url, { headers: { Accept: "application/json" } });
  if (!res.ok) {
    let detail = "";
    try { detail = (await res.json()).error || ""; } catch (_) { /* ignore */ }
    throw new Error(detail || `Request failed (${res.status})`);
  }
  return res.json();
}

/* ---- App state ----------------------------------------------------------- */
const state = {
  matches: [],        // MatchSummary list (newest first)
  match: null,        // current Match
  analysis: null,     // current Analysis
  events: [],         // renderable events (usage filtered out)
  index: 0,           // scrubber position (number of events shown - 1; -1 = none)
  reveal: false,      // reveal private reasoning toggle
  playing: false,
  playTimer: null,
};

const PLAY_INTERVAL_MS = 1200;

/* ===========================================================================
   View / tab switching
   =========================================================================== */
function initTabs() {
  document.querySelectorAll(".tab").forEach((tab) => {
    tab.addEventListener("click", () => switchView(tab.dataset.view));
  });
}

function switchView(view) {
  document.querySelectorAll(".tab").forEach((t) => {
    const active = t.dataset.view === view;
    t.classList.toggle("is-active", active);
    t.setAttribute("aria-selected", active ? "true" : "false");
  });
  $("#view-replay").classList.toggle("is-active", view === "replay");
  $("#view-leaderboard").classList.toggle("is-active", view === "leaderboard");
  if (view === "leaderboard" && !leaderboardLoaded) loadLeaderboard();
}

/* ===========================================================================
   Replay view
   =========================================================================== */
function initReplay() {
  $("#reveal-toggle").addEventListener("change", (e) => {
    state.reveal = e.target.checked;
    renderFeed();
    renderRoster();
    renderResult();
  });

  $("#match-select").addEventListener("change", (e) => {
    if (e.target.value) loadMatch(e.target.value);
  });

  $("#scrub").addEventListener("input", (e) => {
    setIndex(parseInt(e.target.value, 10));
  });

  $("#btn-prev").addEventListener("click", () => { pause(); setIndex(state.index - 1); });
  $("#btn-next").addEventListener("click", () => { pause(); setIndex(state.index + 1); });
  $("#btn-play").addEventListener("click", togglePlay);
}

/** Load the match list and populate the picker. */
async function loadMatches() {
  showReplayStatus("Loading matches…");
  try {
    const data = await fetchJSON("/api/matches");
    state.matches = Array.isArray(data.matches) ? data.matches : [];
  } catch (err) {
    showReplayStatus("Could not load matches: " + err.message, true);
    return;
  }

  if (state.matches.length === 0) {
    showReplayStatus(
      "No matches yet — run `colosseum tournament` to fill the arena.",
      false, true
    );
    return;
  }

  // Populate the <select> picker.
  const select = $("#match-select");
  clear(select);
  for (const m of state.matches) {
    const providers = (m.players || []).map((p) => p.provider).join(" · ");
    const winner = m.winner ? cap(m.winner) + " win" : (m.complete ? "draw" : "in progress");
    const label = `${m.id} — ${providers || "?"} · ${winner} · ${m.rounds || 0} rds`;
    select.appendChild(el("option", { value: m.id, text: label }));
  }

  hideReplayStatus();
  // Auto-load the first (newest) match.
  loadMatch(state.matches[0].id);
}

/** Load a single match + analysis and reset the scrubber. */
async function loadMatch(id) {
  pause();
  $("#match-select").value = id;
  showReplayStatus("Loading replay…");
  let data;
  try {
    data = await fetchJSON("/api/matches/" + encodeURIComponent(id));
  } catch (err) {
    showReplayStatus("Could not load match: " + err.message, true);
    return;
  }

  state.match = data.match || null;
  state.analysis = data.analysis || null;
  // Filter out usage events: they never render in the timeline.
  const rawEvents = (state.match && Array.isArray(state.match.events)) ? state.match.events : [];
  state.events = rawEvents.filter((e) => e && e.type !== "usage");

  if (state.events.length === 0) {
    showReplayStatus("This match has no replayable events.", false, true);
    return;
  }

  hideReplayStatus();
  $("#replay-body").hidden = false;

  // Configure the scrubber over the renderable events.
  const scrub = $("#scrub");
  scrub.min = "0";
  scrub.max = String(state.events.length - 1);
  scrub.value = "0";

  renderRoster();
  renderHighlights();
  renderMetrics();
  setIndex(0); // also renders feed, phase context, counter, result
}

/* ---- Scrubber control ---------------------------------------------------- */
function setIndex(i) {
  const max = state.events.length - 1;
  state.index = Math.max(0, Math.min(i, max));
  $("#scrub").value = String(state.index);
  $("#scrub-counter").textContent = `${state.index + 1} / ${state.events.length}`;
  $("#btn-prev").disabled = state.index <= 0;
  $("#btn-next").disabled = state.index >= max;
  renderFeed();
  renderPhaseContext();
  renderRoster();   // alive/dead status depends on position
  renderResult();
}

function togglePlay() { state.playing ? pause() : play(); }

function play() {
  if (state.index >= state.events.length - 1) setIndex(0); // restart from top if at end
  state.playing = true;
  $("#btn-play").textContent = "⏸";
  $("#btn-play").title = "Pause";
  state.playTimer = setInterval(() => {
    if (state.index >= state.events.length - 1) { pause(); return; }
    setIndex(state.index + 1);
  }, PLAY_INTERVAL_MS);
}

function pause() {
  state.playing = false;
  if (state.playTimer) { clearInterval(state.playTimer); state.playTimer = null; }
  const btn = $("#btn-play");
  if (btn) { btn.textContent = "▶"; btn.title = "Play"; }
}

/* ---- Phase context ("Round N — Night/Day") ------------------------------- */
function currentPhaseInfo() {
  // Walk backward from the current index to the most recent phase_start.
  for (let i = state.index; i >= 0; i--) {
    const e = state.events[i];
    if (e && e.type === "phase_start") {
      return { round: e.round || 0, phase: e.phase || "" };
    }
  }
  // Fall back to the round on the current event if no phase_start yet.
  const cur = state.events[state.index];
  return { round: (cur && cur.round) || 0, phase: (cur && cur.phase) || "" };
}

function renderPhaseContext() {
  const ctx = $("#phase-context");
  const { round, phase } = currentPhaseInfo();
  ctx.classList.remove("night", "day");
  let label = "Pre-game";
  if (phase === "night") { ctx.classList.add("night"); label = `🌙 Round ${round} — Night`; }
  else if (phase === "day") { ctx.classList.add("day"); label = `☀️ Round ${round} — Day`; }
  else if (round) { label = `Round ${round}`; }
  $("#phase-round").textContent = label;
}

/* ===========================================================================
   Event feed rendering — one styled card per renderable event
   =========================================================================== */
function renderFeed() {
  const feed = $("#feed");
  clear(feed);

  // Show every event from the start up to (and including) the current index.
  const visible = state.events.slice(0, state.index + 1);
  let rendered = 0;
  for (const ev of visible) {
    const node = renderEvent(ev);
    if (node) { feed.appendChild(node); rendered++; }
  }
  if (rendered === 0) {
    feed.appendChild(el("div", { class: "feed-empty", text: "Press play or scrub to begin the replay." }));
  } else {
    // Keep the latest event in view as the replay advances.
    feed.lastElementChild.scrollIntoView({ block: "nearest", behavior: "smooth" });
  }
}

/** Returns a feed node for an event, or null if it should not render. */
function renderEvent(ev) {
  switch (ev.type) {
    case "phase_start":  return renderPhase(ev);
    case "speak":        return renderSpeak(ev);
    case "vote":         return renderVote(ev);
    case "night_action": return state.reveal ? renderNightAction(ev) : null; // secret
    case "seer_result":  return state.reveal ? renderSeerResult(ev) : null;  // secret
    case "elimination":  return renderElimination(ev);
    case "protected":
    case "no_death":     return renderInfo(ev);
    case "forfeit":      return renderForfeit(ev);
    // tally / match_start / match_end / usage do not render as feed lines.
    default:             return null;
  }
}

function seatColorVar(actor) {
  const p = state.match && state.match.players && state.match.players.find((x) => x.id === actor);
  return p ? providerColor(p.provider) : DEFAULT_COLOR;
}

function renderPhase(ev) {
  const night = ev.phase === "night";
  const label = night ? `🌙 Round ${ev.round} — Night` : `☀️ Round ${ev.round} — Day`;
  return el("div", { class: "ev-phase " + (night ? "night" : "day"), text: label });
}

function renderSpeak(ev) {
  const card = el("div", {
    class: "ev-speak",
    style: { "--seat": seatColorVar(ev.actor) },
  }, [
    el("div", { class: "who", text: ev.actor || "Unknown" }),
    el("div", { class: "msg", text: ev.message || "" }),
  ]);
  if (state.reveal && ev.reasoning) card.appendChild(reasoningNode(ev.reasoning));
  return card;
}

function renderVote(ev) {
  let line;
  if (ev.target) {
    line = el("div", { class: "ev-vote", style: { "--seat": seatColorVar(ev.actor) } }, [
      el("span", { class: "who", text: ev.actor || "Unknown" }),
      el("span", { class: "arrow", text: "🗳 →" }),
      el("span", { class: "target", text: ev.target }),
    ]);
  } else {
    line = el("div", { class: "ev-vote", style: { "--seat": seatColorVar(ev.actor) } }, [
      el("span", { class: "who", text: ev.actor || "Unknown" }),
      el("span", { class: "abstain", text: "abstains" }),
    ]);
  }
  if (state.reveal && ev.reasoning) line.appendChild(reasoningNode(ev.reasoning));
  return line;
}

function renderNightAction(ev) {
  const action = (ev.data && ev.data.action) || "";
  // The seer "inspect" night_action can be skipped (no target); guard for it.
  let icon = "•", cls = "", text;
  switch (action) {
    case "kill":    icon = "🐺"; cls = "kill";
      text = ev.target ? ` targets ${ev.target}` : " makes no kill"; break;
    case "inspect": icon = "🔮"; cls = "inspect";
      text = ev.target ? ` inspects ${ev.target}` : " skips their vision"; break;
    case "protect": icon = "🛡"; cls = "protect";
      text = ev.target ? ` protects ${ev.target}` : " protects no one"; break;
    default:        text = ev.target ? ` acts on ${ev.target}` : " acts"; break;
  }
  const node = el("div", { class: "ev-secret " + cls, style: { "--seat": seatColorVar(ev.actor) } }, [
    document.createTextNode(icon + " "),
    el("span", { class: "who", text: ev.actor || "Unknown" }),
    document.createTextNode(text),
  ]);
  if (ev.reasoning) node.appendChild(reasoningNode(ev.reasoning));
  return node;
}

function renderSeerResult(ev) {
  const isWolf = !!(ev.data && ev.data.is_werewolf);
  const node = el("div", { class: "ev-secret inspect", style: { "--seat": seatColorVar(ev.actor) } }, [
    document.createTextNode("🔮 "),
    el("span", { class: "who", text: ev.actor || "Seer" }),
    document.createTextNode("'s vision: "),
    el("span", { text: ev.target || "?" }),
    document.createTextNode(" "),
    el("span", {
      class: "verdict " + (isWolf ? "wolf" : "clear"),
      text: isWolf ? "IS a werewolf" : "is NOT a werewolf",
    }),
  ]);
  if (ev.reasoning) node.appendChild(reasoningNode(ev.reasoning));
  return node;
}

function renderElimination(ev) {
  // The public narration already reads well; fall back to a generic line.
  const text = ev.public || `${ev.target || "A player"} was eliminated.`;
  return el("div", { class: "ev-elim" }, [
    el("span", { class: "skull", text: "☠" }),
    document.createTextNode(text),
  ]);
}

function renderInfo(ev) {
  const text = ev.public || (ev.type === "protected" ? "A life was saved in the night." : "No one died.");
  return el("div", { class: "ev-info", text: text });
}

function renderForfeit(ev) {
  const detail = ev.detail ? ` (${ev.detail})` : "";
  return el("div", { class: "ev-forfeit", text: `⚠ ${ev.actor || "A player"} forfeited${detail}` });
}

function reasoningNode(text) {
  return el("div", { class: "reasoning" }, [
    el("span", { class: "thinks", text: "thinks:" }),
    document.createTextNode(text),
  ]);
}

/* ===========================================================================
   Roster — seats with hidden roles + alive/dead status
   =========================================================================== */

/** Set of player ids eliminated at or before the current scrubber index. */
function deadByNow() {
  const dead = new Set();
  for (let i = 0; i <= state.index && i < state.events.length; i++) {
    const e = state.events[i];
    if (e && e.type === "elimination" && e.target) dead.add(e.target);
  }
  return dead;
}

function renderRoster() {
  const list = $("#roster");
  clear(list);
  const players = (state.match && state.match.players) || [];
  const dead = deadByNow();

  for (const p of players) {
    const isDead = dead.has(p.id);
    const seat = el("li", {
      class: "seat" + (isDead ? " dead" : ""),
      style: { "--seat": providerColor(p.provider) },
    }, [
      el("span", { class: "dot" }),
      el("div", { class: "seat-main" }, [
        el("div", { class: "seat-id", text: p.id }),
        el("div", { class: "seat-model", text: `${p.provider || "?"} · ${p.model || "?"}` }),
      ]),
      roleBadge(p.role),
    ]);
    list.appendChild(seat);
  }
}

/** Role badge: hidden ("???") unless the reveal toggle is on. */
function roleBadge(role) {
  if (!state.reveal) {
    return el("span", { class: "seat-role role-hidden", text: "hidden" });
  }
  const r = role || "villager";
  return el("span", { class: "seat-role role-" + r, text: r });
}

/* ===========================================================================
   Result banner — who won
   =========================================================================== */
function renderResult() {
  const banner = $("#result-banner");
  const m = state.match;
  if (!m) { banner.hidden = true; return; }

  // Only reveal the outcome once the scrubber reaches the end (the dramatic
  // payoff), or immediately if the match is incomplete.
  const atEnd = state.index >= state.events.length - 1;
  if (!atEnd && m.complete) {
    banner.hidden = true;
    return;
  }

  banner.hidden = false;
  banner.className = "result-banner";

  if (!m.complete) {
    clear(banner);
    banner.appendChild(document.createTextNode("⏳ Match incomplete"));
    return;
  }

  const winner = m.winner || "";
  clear(banner);
  if (winner === "village") {
    banner.classList.add("village");
    banner.appendChild(document.createTextNode("🏘 Village wins"));
  } else if (winner === "werewolf") {
    banner.classList.add("werewolf");
    banner.appendChild(document.createTextNode("🐺 Werewolves win"));
  } else {
    banner.appendChild(document.createTextNode("Draw"));
  }
  const survivors = (m.survivors || []).join(", ");
  banner.appendChild(el("span", {
    class: "sub",
    text: survivors ? `Survivors: ${survivors}` : "No survivors",
  }));
}

/* ===========================================================================
   Highlights panel
   =========================================================================== */
const HIGHLIGHT_ICONS = {
  wolf_victory: "🐺",
  lone_wolf: "🌑",
  seer_ignored: "🔮",
  mislynch_power_role: "⚖️",
  doctor_save: "🛡",
  wolf_survived_close_vote: "🔪",
  flawless_village: "🏘",
};

function renderHighlights() {
  const wrap = $("#highlights");
  clear(wrap);
  const highlights = (state.analysis && state.analysis.highlights) || [];
  if (highlights.length === 0) {
    wrap.appendChild(el("div", { class: "empty", text: "No standout moments detected." }));
    return;
  }
  for (const h of highlights) {
    const icon = HIGHLIGHT_ICONS[h.type] || "✨";
    const card = el("div", { class: "hl-card", title: "Jump to this round" }, [
      el("div", { class: "hl-icon", text: icon }),
      el("div", { class: "hl-body" }, [
        h.round ? el("div", { class: "hl-round", text: "Round " + h.round }) : null,
        el("div", { class: "hl-title", text: h.title || "Highlight" }),
        h.detail ? el("div", { class: "hl-detail", text: h.detail }) : null,
      ]),
    ]);
    // Clicking a highlight jumps the scrubber to that round's first event.
    if (h.round) card.addEventListener("click", () => jumpToRound(h.round));
    wrap.appendChild(card);
  }
}

/** Move the scrubber to the first event of the given round. */
function jumpToRound(round) {
  pause();
  for (let i = 0; i < state.events.length; i++) {
    if (state.events[i].round === round) { setIndex(i); return; }
  }
}

/* ===========================================================================
   Metrics panel — deception / deduction / persuasion per player
   =========================================================================== */
function renderMetrics() {
  const wrap = $("#metrics");
  clear(wrap);
  const players = (state.analysis && state.analysis.players) || [];
  if (players.length === 0) {
    wrap.appendChild(el("div", { class: "empty", text: "No metrics available." }));
    return;
  }
  for (const p of players) {
    const row = el("div", { class: "metric-row" }, [
      el("div", { class: "metric-head", style: { "--seat": providerColor(p.provider) } }, [
        el("span", { class: "dot" }),
        el("span", { class: "id", text: p.id }),
        el("span", {
          class: "outcome " + (p.won ? "won" : "lost"),
          text: p.won ? "won" : "lost",
        }),
      ]),
      el("div", { class: "metric-bars" }, [
        ...metricBar("Deception", p.deception),
        ...metricBar("Deduction", p.deduction),
        ...metricBar("Persuasion", p.persuasion),
      ]),
    ]);
    wrap.appendChild(row);
  }
}

/** Builds the 3 grid cells for one metric. value is a float 0..1, or null/undefined. */
function metricBar(label, value) {
  const has = typeof value === "number" && isFinite(value);
  const pct = has ? Math.round(Math.max(0, Math.min(1, value)) * 100) : 0;
  const track = el("div", { class: "track" }, [
    el("div", { class: "fill", style: { width: has ? pct + "%" : "0%" } }),
  ]);
  return [
    el("span", { class: "label", text: label }),
    track,
    el("span", { class: "val" + (has ? "" : " na"), text: has ? value.toFixed(2) : "—" }),
  ];
}

/* ---- Replay status helpers ----------------------------------------------- */
function showReplayStatus(msg, isError = false, isEmpty = false) {
  const panel = $("#replay-status");
  $("#replay-body").hidden = true;
  panel.hidden = false;
  panel.className = "status-panel" + (isError ? " error" : "");
  clear(panel);
  if (isEmpty) {
    // Render the hint with a styled <code> for the command.
    panel.appendChild(document.createTextNode("No matches yet — run "));
    panel.appendChild(el("code", { text: "colosseum tournament" }));
    panel.appendChild(document.createTextNode(" to fill the arena."));
  } else {
    panel.textContent = msg;
  }
}
function hideReplayStatus() { $("#replay-status").hidden = true; }

/* ===========================================================================
   Leaderboard view
   =========================================================================== */
let leaderboardLoaded = false;
let leaderboardRows = [];
const leaderboardSort = { key: "rank", dir: 1 }; // dir: 1 asc, -1 desc

// Column definitions: { key, label, format, defaultDir }
const LB_COLUMNS = [
  { key: "rank",          label: "Rank",       fmt: (r) => r.rank,                 dir: 1 },
  { key: "model",         label: "Model",      fmt: (r) => r.model,                dir: 1 },
  { key: "provider",      label: "Provider",   fmt: (r) => r.provider,             dir: 1 },
  { key: "elo",           label: "ELO",        fmt: (r) => Math.round(r.elo),      dir: -1 },
  { key: "wl",            label: "W-L",        fmt: (r) => `${r.wins}-${r.losses}`,dir: -1, sortVal: (r) => r.wins - r.losses },
  { key: "win_rate",      label: "Win%",       fmt: (r) => pct(r.win_rate),        dir: -1 },
  { key: "wolf_win_rate", label: "Wolf Win%",  fmt: (r) => pct(r.wolf_win_rate),   dir: -1 },
  { key: "deception",     label: "Deception",  fmt: (r) => num2(r.deception),      dir: -1 },
  { key: "deduction",     label: "Deduction",  fmt: (r) => num2(r.deduction),      dir: -1 },
  { key: "persuasion",    label: "Persuasion", fmt: (r) => num2(r.persuasion),     dir: -1 },
];

async function loadLeaderboard() {
  leaderboardLoaded = true;
  showLeaderboardStatus("Loading leaderboard…");
  let data;
  try {
    data = await fetchJSON("/api/leaderboard");
  } catch (err) {
    showLeaderboardStatus("Could not load leaderboard: " + err.message, true);
    leaderboardLoaded = false; // allow retry on next tab switch
    return;
  }
  leaderboardRows = Array.isArray(data.standings) ? data.standings : [];

  if (leaderboardRows.length === 0) {
    showLeaderboardStatus(
      "No matches recorded yet — run `colosseum tournament` first.",
      false, true
    );
    return;
  }

  hideLeaderboardStatus();
  $("#leaderboard-wrap").hidden = false;
  renderLeaderboardHead();
  renderLeaderboardBody();
}

function renderLeaderboardHead() {
  const head = $("#leaderboard-head");
  clear(head);
  for (const col of LB_COLUMNS) {
    const active = leaderboardSort.key === col.key;
    const ind = active ? (leaderboardSort.dir === 1 ? " ▲" : " ▼") : "";
    const th = el("th", { title: "Sort by " + col.label }, [
      document.createTextNode(col.label),
      active ? el("span", { class: "sort-ind", text: ind }) : null,
    ]);
    th.addEventListener("click", () => sortLeaderboard(col));
    head.appendChild(th);
  }
}

function sortLeaderboard(col) {
  if (leaderboardSort.key === col.key) {
    leaderboardSort.dir *= -1; // toggle direction
  } else {
    leaderboardSort.key = col.key;
    leaderboardSort.dir = col.dir || 1;
  }
  const getVal = col.sortVal || ((r) => r[col.key]);
  const dir = leaderboardSort.dir;
  leaderboardRows.sort((a, b) => {
    const va = getVal(a), vb = getVal(b);
    if (typeof va === "string" || typeof vb === "string") {
      return String(va).localeCompare(String(vb)) * dir;
    }
    return ((va || 0) - (vb || 0)) * dir;
  });
  renderLeaderboardHead();
  renderLeaderboardBody();
}

function renderLeaderboardBody() {
  const body = $("#leaderboard-body");
  clear(body);
  for (const r of leaderboardRows) {
    const isTop = r.rank === 1;
    const tr = el("tr", { class: isTop ? "top" : "" });
    tr.appendChild(el("td", {}, el("span", { class: "rank", text: String(r.rank) })));
    tr.appendChild(el("td", {}, el("span", { class: "model", text: r.model || "?" })));
    tr.appendChild(el("td", {}, el("span", {
      class: "provider-tag",
      style: { "--seat": providerColor(r.provider) },
      text: r.provider || "?",
    })));
    tr.appendChild(el("td", { text: String(Math.round(r.elo || 0)) }));
    tr.appendChild(el("td", { text: `${r.wins || 0}-${r.losses || 0}` }));
    tr.appendChild(el("td", { text: pct(r.win_rate) }));
    tr.appendChild(el("td", { text: pct(r.wolf_win_rate) }));
    tr.appendChild(el("td", { text: num2(r.deception) }));
    tr.appendChild(el("td", { text: num2(r.deduction) }));
    tr.appendChild(el("td", { text: num2(r.persuasion) }));
    body.appendChild(tr);
  }
}

function showLeaderboardStatus(msg, isError = false, isEmpty = false) {
  const panel = $("#leaderboard-status");
  $("#leaderboard-wrap").hidden = true;
  panel.hidden = false;
  panel.className = "status-panel" + (isError ? " error" : "");
  clear(panel);
  if (isEmpty) {
    panel.appendChild(document.createTextNode("No matches recorded yet — run "));
    panel.appendChild(el("code", { text: "colosseum tournament" }));
    panel.appendChild(document.createTextNode(" first."));
  } else {
    panel.textContent = msg;
  }
}
function hideLeaderboardStatus() { $("#leaderboard-status").hidden = true; }

/* ---- Formatting helpers -------------------------------------------------- */
function pct(v) {
  return (typeof v === "number" && isFinite(v)) ? Math.round(v * 100) + "%" : "—";
}
function num2(v) {
  return (typeof v === "number" && isFinite(v)) ? v.toFixed(2) : "—";
}
function cap(s) { return s ? s.charAt(0).toUpperCase() + s.slice(1) : s; }

/* ===========================================================================
   Boot
   =========================================================================== */
function init() {
  initTabs();
  initReplay();
  loadMatches();
}

if (document.readyState === "loading") {
  document.addEventListener("DOMContentLoaded", init);
} else {
  init();
}
