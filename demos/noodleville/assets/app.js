const canvas = document.querySelector("#map");
const ctx = canvas.getContext("2d");
const feedEl = document.querySelector("#feed");
const clockEl = document.querySelector("#clock");
const villagersEl = document.querySelector("#villagers");
const memoriesEl = document.querySelector("#memories");
const partyEl = document.querySelector("#party");
const inspectNameEl = document.querySelector("#inspect-name");
const inspectRoleEl = document.querySelector("#inspect-role");
const inspectPlanEl = document.querySelector("#inspect-plan");
const inspectSourceEl = document.querySelector("#inspect-source");
const partyBadgeEl = document.querySelector("#party-badge");
const relationshipsEl = document.querySelector("#relationships");
const inspectorMemoriesEl = document.querySelector("#inspector-memories");

let state = { snapshot: null, feed: [], memory_counts: {} };
let selectedVillagerID = "maya";
let inspector = null;
let hitRegions = [];

const colors = ["#b54832", "#2e7d4f", "#2f5f9f", "#a9781d", "#7e4f9a", "#2b7c82", "#9a3e62"];

function draw() {
  const snapshot = state.snapshot;
  if (!snapshot) return;
  const w = canvas.width;
  const h = canvas.height;
  const cell = Math.min(w / snapshot.width, h / snapshot.height);
  hitRegions = [];
  ctx.clearRect(0, 0, w, h);
  ctx.fillStyle = "#ece1c1";
  ctx.fillRect(0, 0, w, h);

  ctx.strokeStyle = "#503d2f";
  ctx.lineWidth = 2;
  for (let x = 0; x <= snapshot.width; x++) {
    ctx.beginPath();
    ctx.moveTo(x * cell, 0);
    ctx.lineTo(x * cell, snapshot.height * cell);
    ctx.stroke();
  }
  for (let y = 0; y <= snapshot.height; y++) {
    ctx.beginPath();
    ctx.moveTo(0, y * cell);
    ctx.lineTo(snapshot.width * cell, y * cell);
    ctx.stroke();
  }

  snapshot.places.forEach((place) => {
    const cx = place.position.x * cell + cell / 2;
    const cy = place.position.y * cell + cell / 2;
    ctx.fillStyle = place.kind === "house" ? "#cfb77a" : "#789f5c";
    if (place.kind === "cafe" || place.kind === "bakery") ctx.fillStyle = "#d88b42";
    if (place.kind === "stage") ctx.fillStyle = "#b54832";
    ctx.fillRect(cx - cell * 0.22, cy - cell * 0.22, cell * 0.44, cell * 0.44);
    ctx.fillStyle = "#3a2c22";
    ctx.font = "700 14px ui-sans-serif, system-ui";
    ctx.textAlign = "center";
    ctx.fillText(place.name.split(" ")[0], cx, cy + cell * 0.42);
  });

  const groups = new Map();
  snapshot.villagers.forEach((villager) => {
    const key = `${villager.position.x},${villager.position.y}`;
    if (!groups.has(key)) groups.set(key, []);
    groups.get(key).push(villager);
  });

  [...groups.values()].forEach((group) => {
    group.forEach((villager, i) => {
      const total = group.length;
      const angle = (Math.PI * 2 * i) / Math.max(total, 1);
      const radius = total > 1 ? cell * 0.18 : 0;
      const cx = villager.position.x * cell + cell / 2 + Math.cos(angle) * radius;
      const cy = villager.position.y * cell + cell / 2 + Math.sin(angle) * radius;
      const colorIndex = Math.abs(hash(villager.profile.id)) % colors.length;
      const party = snapshot.social?.party?.[villager.profile.id];
      const selected = villager.profile.id === selectedVillagerID;
      ctx.beginPath();
      ctx.arc(cx, cy, cell * 0.14, 0, Math.PI * 2);
      ctx.fillStyle = colors[colorIndex];
      ctx.fill();
      ctx.lineWidth = selected ? 6 : 3;
      ctx.strokeStyle = selected ? "#16100c" : "#2e251d";
      ctx.stroke();
      if (party?.knows) {
        ctx.beginPath();
        ctx.arc(cx, cy, cell * 0.2, 0, Math.PI * 2);
        ctx.strokeStyle = "#d88b42";
        ctx.lineWidth = 4;
        ctx.stroke();
      }
      ctx.fillStyle = "#fff7dc";
      ctx.font = "800 13px ui-sans-serif, system-ui";
      ctx.textAlign = "center";
      ctx.textBaseline = "middle";
      ctx.fillText(villager.profile.name.slice(0, 1), cx, cy);
      hitRegions.push({ id: villager.profile.id, x: cx, y: cy, r: cell * 0.24 });
    });
  });
}

function render() {
  const snapshot = state.snapshot;
  if (!snapshot) return;
  clockEl.textContent = `day ${snapshot.clock.day} ${String(Math.floor(snapshot.clock.minute / 60)).padStart(2, "0")}:${String(snapshot.clock.minute % 60).padStart(2, "0")}`;
  villagersEl.textContent = snapshot.villagers.length;
  memoriesEl.textContent = Object.values(state.memory_counts || {}).reduce((sum, value) => sum + value, 0);
  partyEl.textContent = Object.values(snapshot.social?.party || {}).filter((knowledge) => knowledge.knows).length;
  feedEl.replaceChildren(...state.feed.slice(-80).reverse().map(renderEvent));
  renderInspector();
  draw();
}

function renderInspector() {
  const snapshot = state.snapshot;
  const villager = snapshot?.villagers.find((v) => v.profile.id === selectedVillagerID);
  if (!villager) return;
  const party = snapshot.social?.party?.[selectedVillagerID] || { knows: false };
  inspectNameEl.textContent = villager.profile.name;
  inspectRoleEl.textContent = `${villager.profile.role}; ${villager.profile.mood}`;
  inspectPlanEl.textContent = villager.current_plan || villager.profile.goal;
  inspectSourceEl.textContent = party.knows ? `${party.source_name || "self"} - ${party.evidence || "known"}` : "Has not heard it yet";
  partyBadgeEl.textContent = party.knows ? "knows party" : "unknown";
  partyBadgeEl.dataset.state = party.knows ? "known" : "unknown";

  const rels = Object.values(snapshot.social?.relationships?.[selectedVillagerID] || {})
    .sort((a, b) => b.conversations - a.conversations || b.familiarity - a.familiarity)
    .slice(0, 4);
  relationshipsEl.replaceChildren(...listItems(rels, (rel) => `${rel.villager_name}: ${rel.conversations} talks, trust ${rel.trust}`));

  const memories = inspector?.villager?.profile?.id === selectedVillagerID ? inspector.memories || [] : [];
  inspectorMemoriesEl.replaceChildren(...listItems(memories.slice(0, 3), (memory) => memory.text));
}

function renderEvent(event) {
  const item = document.createElement("li");
  const time = document.createElement("time");
  time.textContent = `d${event.at.day} ${String(Math.floor(event.at.minute / 60)).padStart(2, "0")}:${String(event.at.minute % 60).padStart(2, "0")}`;
  const text = document.createElement("p");
  text.textContent = event.text;
  item.append(time, text);
  return item;
}

async function loadInitial() {
  const response = await fetch("/api/state");
  state = await response.json();
  const villagers = state.snapshot?.villagers || [];
  selectedVillagerID = villagers.some((villager) => villager.profile.id === "maya") ? "maya" : villagers[0]?.profile?.id || selectedVillagerID;
  await loadInspector(selectedVillagerID);
  render();
}

function connect() {
  const source = new EventSource("/api/events");
  source.addEventListener("tick", (event) => {
    const report = JSON.parse(event.data);
    if (report.snapshot) state.snapshot = report.snapshot;
    else state.snapshot.clock = report.ended_at;
    state.feed = [...state.feed, ...report.events];
    if (report.memory_counts) state.memory_counts = report.memory_counts;
    render();
    loadInspector(selectedVillagerID).then(render).catch(() => {});
  });
}

async function loadInspector(id) {
  const response = await fetch(`/api/inspect?id=${encodeURIComponent(id)}`);
  if (response.ok) inspector = await response.json();
}

function listItems(items, text) {
  if (!items.length) {
    const empty = document.createElement("li");
    empty.textContent = "No entries yet";
    return [empty];
  }
  return items.map((item) => {
    const li = document.createElement("li");
    li.textContent = text(item);
    return li;
  });
}

canvas.addEventListener("click", async (event) => {
  const rect = canvas.getBoundingClientRect();
  const x = ((event.clientX - rect.left) / rect.width) * canvas.width;
  const y = ((event.clientY - rect.top) / rect.height) * canvas.height;
  const hit = hitRegions.find((region) => Math.hypot(region.x - x, region.y - y) <= region.r);
  if (!hit) return;
  selectedVillagerID = hit.id;
  await loadInspector(selectedVillagerID);
  render();
});

canvas.addEventListener("mousemove", (event) => {
  const rect = canvas.getBoundingClientRect();
  const x = ((event.clientX - rect.left) / rect.width) * canvas.width;
  const y = ((event.clientY - rect.top) / rect.height) * canvas.height;
  canvas.style.cursor = hitRegions.some((region) => Math.hypot(region.x - x, region.y - y) <= region.r) ? "pointer" : "default";
});

function hash(value) {
  let h = 0;
  for (let i = 0; i < value.length; i++) h = (h << 5) - h + value.charCodeAt(i);
  return h;
}

loadInitial().then(connect).catch((error) => {
  feedEl.innerHTML = `<li><time>error</time><p>${error.message}</p></li>`;
});
