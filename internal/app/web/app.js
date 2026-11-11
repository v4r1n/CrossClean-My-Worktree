"use strict";

// ---- API -------------------------------------------------------------------

// The server prints a link with a per-launch token in the URL fragment. Keep it
// for this tab only, and take a new one when the link is opened again.
const TOKEN_KEY = "cleanmywt-token";
function readToken() {
  const fromHash = location.hash.slice(1);
  if (fromHash) {
    try { sessionStorage.setItem(TOKEN_KEY, fromHash); } catch { /* storage blocked */ }
    history.replaceState(null, "", location.pathname);
    return fromHash;
  }
  try { return sessionStorage.getItem(TOKEN_KEY) || ""; } catch { return ""; }
}
let token = readToken();
addEventListener("hashchange", () => { token = readToken(); refresh(); });

class ApiError extends Error {
  constructor(message, status) { super(message); this.status = status; }
}

async function api(path, body) {
  const res = await fetch("/api/" + path, {
    method: body === undefined ? "GET" : "POST",
    headers: { "X-CleanMyWT-Token": token, "Content-Type": "application/json" },
    body: body === undefined ? undefined : JSON.stringify(body),
  });
  const data = await res.json().catch(() => ({}));
  if (!res.ok) throw new ApiError(data.error || res.statusText, res.status);
  return data;
}

// ---- State -----------------------------------------------------------------

const ui = {
  snap: null,
  checked: new Set(),
  repo: "all",
  filter: "all",
  search: "",
  sort: { key: "repo", dir: 1 },
  lastClicked: -1,
  shown: [],
  menuPath: null,
  dialog: null, // {stage, items, jobId}
};

const $ = (id) => document.getElementById(id);
const esc = (s) => String(s ?? "").replace(/[&<>"']/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;", '"': "&quot;", "'": "&#39;" }[c]));

function formatBytes(n) {
  if (n == null) return "—";
  if (n < 1000) return n === 1 ? "1 byte" : `${n} bytes`;
  const units = ["KB", "MB", "GB", "TB", "PB"], decimals = [0, 1, 2, 2, 2];
  let v = n, i = -1;
  while (v >= 1000 && i < units.length - 1) { v /= 1000; i++; }
  return `${v.toFixed(decimals[i])} ${units[i]}`;
}

const rtf = new Intl.RelativeTimeFormat(undefined, { numeric: "auto", style: "short" });
function relative(iso) {
  if (!iso) return "—";
  const secs = (new Date(iso) - Date.now()) / 1000;
  const steps = [[60, "second"], [3600, "minute"], [86400, "hour"], [604800, "day"], [2629800, "week"], [31557600, "month"], [Infinity, "year"]];
  const sizes = { second: 1, minute: 60, hour: 3600, day: 86400, week: 604800, month: 2629800, year: 31557600 };
  for (const [limit, unit] of steps) {
    if (Math.abs(secs) < limit) return rtf.format(Math.round(secs / sizes[unit]), unit);
  }
  return "—";
}

const verdictIcon = { safe: "✔", review: "?", risky: "⚠", pending: "…" };
const verdictTitle = {
  safe: "Safe to delete: no uncommitted or unpushed work",
  review: "Check before deleting",
  risky: "Deleting loses work or disrupts something running",
  pending: "Still checking",
};
const isBusy = () => ui.snap && (ui.snap.progress.phase !== "idle" || (ui.snap.job && ui.snap.job.running));

// ---- Polling ---------------------------------------------------------------

let pollTimer = null;
async function refresh() {
  clearTimeout(pollTimer);
  try {
    ui.snap = await api("state");
    render();
  } catch (err) {
    $("phase").textContent = !token || err.status === 403
      ? "This tab isn't connected. Open the link that cleanmywt printed in its terminal window."
      : "Lost connection to CleanMyWT. Is it still running?";
  }
  pollTimer = setTimeout(refresh, isBusy() ? 600 : 5000);
}

// ---- Rendering -------------------------------------------------------------

function render() {
  const snap = ui.snap;
  const paths = new Set(snap.rows.map((r) => r.path));
  for (const p of ui.checked) if (!paths.has(p)) ui.checked.delete(p);
  if (ui.repo !== "all" && !snap.repos.some((r) => r.id === ui.repo)) ui.repo = "all";

  renderToolbar(snap);
  renderRepos(snap);
  renderRows(snap);
  renderSummary(snap);
  if (ui.dialog) renderDialog();
}

function targetKey(t) { return t.kind === "folder" ? "folder:" + t.path : t.kind; }

function renderToolbar(snap) {
  const select = $("target");
  if (document.activeElement !== select) {
    const current = targetKey(snap.target);
    select.innerHTML = snap.targets.map((o) => `<option value="${esc(targetKey(o.target))}">${esc(o.title)}</option>`).join("")
      + `<option value="choose">Choose folder…</option>`;
    select.value = current;
  }
  const p = snap.progress;
  let text = "";
  if (p.phase === "scanning") text = p.detail || "Scanning…";
  else if (p.phase === "inspecting") text = `Checking git status ${p.done} of ${p.total}…`;
  else if (p.phase === "measuring") text = `Measuring disk usage ${p.done} of ${p.total}…`;
  else if (snap.lastScan) text = `Scanned ${relative(snap.lastScan)}`;
  $("phase").textContent = text;
  $("stop").hidden = p.phase === "idle";
  $("rescan").disabled = !!(snap.job && snap.job.running);
  $("free").textContent = snap.freeSpace != null ? `${formatBytes(snap.freeSpace)} free` : "";
}

function renderRepos(snap) {
  const total = snap.rows.reduce((s, r) => s + (r.usage ? r.usage.allocated : 0), 0);
  const items = [{ id: "all", name: "All repositories", count: snap.rows.length, bytes: total }, ...snap.repos];
  $("repos").innerHTML = items.map((r) => `
    <li data-repo="${esc(r.id)}" class="${r.id === ui.repo ? "active" : ""}" title="${esc(r.id === "all" ? "" : r.id)}">
      <span class="name">${esc(r.name)}</span><span>${r.count}</span>
      <span class="meta">${r.bytes ? formatBytes(r.bytes) : ""}</span>
    </li>`).join("");
}

const sorters = {
  verdict: (r) => ({ safe: 0, review: 1, risky: 2, pending: 3 })[r.verdict],
  name: (r) => r.name.toLowerCase(),
  repo: (r) => r.repoName.toLowerCase() + "\u0000" + r.path,
  branch: (r) => r.branchLabel.toLowerCase(),
  activity: (r) => (r.lastActivity ? Date.parse(r.lastActivity) : 0),
  size: (r) => (r.usage ? r.usage.allocated : -1),
  freed: (r) => (r.usage ? r.usage.reclaimable : -1),
};

function visibleRows(snap) {
  const q = ui.search.trim().toLowerCase();
  const rows = snap.rows.filter((r) =>
    (ui.repo === "all" || r.repoId === ui.repo) &&
    (ui.filter === "all" || (ui.filter === "safe" ? r.verdict === "safe" : r.verdict === "review" || r.verdict === "risky")) &&
    (!q || [r.name, r.repoName, r.branchLabel, r.path].some((s) => s.toLowerCase().includes(q))));
  const key = sorters[ui.sort.key];
  return rows.sort((a, b) => {
    const x = key(a), y = key(b);
    return (x < y ? -1 : x > y ? 1 : 0) * ui.sort.dir;
  });
}

function renderRows(snap) {
  ui.shown = visibleRows(snap);
  document.querySelectorAll("th.sortable").forEach((th) => {
    th.classList.toggle("sorted-asc", th.dataset.sort === ui.sort.key && ui.sort.dir === 1);
    th.classList.toggle("sorted-desc", th.dataset.sort === ui.sort.key && ui.sort.dir === -1);
  });
  $("rows").innerHTML = ui.shown.map((r, i) => `
    <tr data-i="${i}" class="${ui.checked.has(r.path) ? "checked" : ""}">
      <td><input type="checkbox" class="mark" data-i="${i}" ${ui.checked.has(r.path) ? "checked" : ""} aria-label="Mark ${esc(r.name)}"></td>
      <td class="verdict verdict-${r.verdict}" title="${esc(verdictTitle[r.verdict])}">${verdictIcon[r.verdict]}</td>
      <td title="${esc(r.path)}"><div class="wt-name">${esc(r.name)}</div><div class="wt-path">${esc(r.display)}</div></td>
      <td>${esc(r.repoName)}</td>
      <td><div class="branch" title="${esc(r.branchLabel)}">${esc(r.branchLabel)}</div></td>
      <td title="${esc(r.detail)}"><div class="badges">${r.badges.map((b) => `<span class="badge tone-${b.tone}">${esc(b.text)}</span>`).join("")}</div></td>
      <td class="num">${relative(r.lastActivity)}</td>
      <td class="num">${r.usage ? formatBytes(r.usage.allocated) : r.prunable ? "—" : "…"}</td>
      <td class="num">${r.usage ? formatBytes(r.usage.reclaimable) : r.prunable ? "—" : "…"}</td>
      <td><button class="more" data-i="${i}" title="More actions" aria-label="More actions">⋯</button></td>
    </tr>`).join("");

  const empty = $("empty");
  empty.hidden = ui.shown.length > 0;
  if (!ui.shown.length) {
    empty.textContent = snap.rows.length ? "No worktrees match the filter."
      : snap.progress.phase !== "idle" ? "Looking for worktrees…" : "No git worktrees found here.";
  }
  const all = $("check-all");
  const marked = ui.shown.filter((r) => ui.checked.has(r.path)).length;
  all.checked = ui.shown.length > 0 && marked === ui.shown.length;
  all.indeterminate = marked > 0 && marked < ui.shown.length;
}

function renderSummary(snap) {
  const marked = snap.rows.filter((r) => ui.checked.has(r.path));
  const total = snap.rows.reduce((s, r) => s + (r.usage ? r.usage.allocated : 0), 0);
  const safe = snap.rows.filter((r) => r.verdict === "safe");
  $("summary").textContent = marked.length
    ? `${marked.length} marked · frees ${formatBytes(marked.reduce((s, r) => s + r.bytesFreed, 0))}`
    : `${snap.rows.length} worktrees · ${formatBytes(total)} on disk · ${safe.length} safe to delete`;
  $("delete").disabled = !marked.length || isBusy();
  $("mark-safe").disabled = !ui.shown.some((r) => r.verdict === "safe");
  $("clear-marks").disabled = !marked.length;
}

// ---- Toolbar & filters -----------------------------------------------------

function scan(target) {
  api("scan", target).then(refresh).catch((err) => toast(err.message));
}

$("target").addEventListener("change", (e) => {
  const value = e.target.value;
  if (value === "choose") {
    $("folder-form").hidden = false;
    $("folder-input").focus();
    e.target.value = targetKey(ui.snap.target);
    return;
  }
  const option = ui.snap.targets.find((o) => targetKey(o.target) === value);
  if (option) scan(option.target);
});

$("folder-form").addEventListener("submit", (e) => {
  e.preventDefault();
  const path = $("folder-input").value.trim();
  if (!path) return;
  $("folder-form").hidden = true;
  scan({ kind: "folder", path });
});

$("rescan").addEventListener("click", () => scan(ui.snap.target));
$("stop").addEventListener("click", () => api("stop", {}).then(refresh));

$("repos").addEventListener("click", (e) => {
  const li = e.target.closest("li");
  if (li) { ui.repo = li.dataset.repo; ui.lastClicked = -1; render(); }
});

$("verdict-filter").addEventListener("click", (e) => {
  const btn = e.target.closest("button");
  if (!btn) return;
  ui.filter = btn.dataset.filter;
  ui.lastClicked = -1;
  document.querySelectorAll("#verdict-filter button").forEach((b) => b.classList.toggle("active", b === btn));
  render();
});

$("search").addEventListener("input", (e) => { ui.search = e.target.value; ui.lastClicked = -1; render(); });

document.querySelector("thead").addEventListener("click", (e) => {
  const th = e.target.closest("th.sortable");
  if (!th) return;
  const key = th.dataset.sort;
  const descFirst = ["size", "freed", "activity"].includes(key);
  ui.sort = ui.sort.key === key ? { key, dir: -ui.sort.dir } : { key, dir: descFirst ? -1 : 1 };
  render();
});

// ---- Marking ---------------------------------------------------------------

$("rows").addEventListener("click", (e) => {
  const box = e.target.closest("input.mark");
  if (box) {
    const i = Number(box.dataset.i);
    const value = box.checked;
    if (e.shiftKey && ui.lastClicked >= 0) {
      const [a, b] = [Math.min(ui.lastClicked, i), Math.max(ui.lastClicked, i)];
      for (let k = a; k <= b; k++) setChecked(ui.shown[k].path, value);
    } else {
      setChecked(ui.shown[i].path, value);
    }
    ui.lastClicked = i;
    render();
    return;
  }
  const more = e.target.closest("button.more");
  if (more) openMenu(more, ui.shown[Number(more.dataset.i)].path);
});

$("rows").addEventListener("contextmenu", (e) => {
  const tr = e.target.closest("tr");
  if (!tr) return;
  e.preventDefault();
  openMenu({ getBoundingClientRect: () => ({ left: e.clientX, bottom: e.clientY }) }, ui.shown[Number(tr.dataset.i)].path);
});

function setChecked(path, value) { value ? ui.checked.add(path) : ui.checked.delete(path); }

$("check-all").addEventListener("change", (e) => {
  for (const r of ui.shown) setChecked(r.path, e.target.checked);
  render();
});
$("mark-safe").addEventListener("click", () => {
  for (const r of ui.shown) if (r.verdict === "safe") ui.checked.add(r.path);
  render();
});
$("clear-marks").addEventListener("click", () => { ui.checked.clear(); render(); });

// ---- Row menu --------------------------------------------------------------

function openMenu(anchor, path) {
  const menu = $("menu");
  const rect = anchor.getBoundingClientRect();
  ui.menuPath = path;
  menu.hidden = false;
  menu.style.left = Math.min(rect.left, innerWidth - menu.offsetWidth - 8) + "px";
  menu.style.top = Math.min(rect.bottom, innerHeight - menu.offsetHeight - 8) + "px";
}

$("menu").addEventListener("click", async (e) => {
  const btn = e.target.closest("button");
  if (!btn) return;
  const path = ui.menuPath;
  $("menu").hidden = true;
  try {
    if (btn.dataset.action === "copy") {
      await navigator.clipboard.writeText(path);
      toast("Path copied");
    } else {
      await api("open", { path, action: btn.dataset.action });
    }
  } catch (err) { toast(err.message); }
});

document.addEventListener("click", (e) => {
  if (!e.target.closest("#menu") && !e.target.closest("button.more")) $("menu").hidden = true;
});

let toastTimer = null;
function toast(text) {
  const t = $("toast");
  t.textContent = text;
  t.hidden = false;
  clearTimeout(toastTimer);
  toastTimer = setTimeout(() => { t.hidden = true; }, 2500);
}

// ---- Keyboard --------------------------------------------------------------

document.addEventListener("keydown", (e) => {
  const typing = ["INPUT", "SELECT", "TEXTAREA"].includes(document.activeElement.tagName)
    && document.activeElement.type !== "checkbox";
  if ((e.ctrlKey || e.metaKey) && e.key.toLowerCase() === "r" && ui.snap) {
    e.preventDefault();
    scan(ui.snap.target);
  } else if (e.key === "Escape") {
    $("menu").hidden = true;
    $("folder-form").hidden = true;
  } else if ((e.key === "Delete" || ((e.metaKey || e.ctrlKey) && e.key === "Backspace")) && !typing && !$("delete").disabled) {
    openDeleteDialog();
  }
});

// ---- Delete dialog ---------------------------------------------------------

$("delete").addEventListener("click", openDeleteDialog);

function openDeleteDialog() {
  const items = ui.snap.rows.filter((r) => ui.checked.has(r.path));
  if (!items.length) return;
  ui.dialog = { stage: "confirm", items, jobId: null, ack: false, branches: false };
  renderDialog();
  $("dialog").showModal();
}

const plural = (n, one, many) => `${n} ${n === 1 ? one : many}`;

function renderDialog() {
  const d = ui.dialog;
  const job = ui.snap.job;
  if (d.stage === "running" && job && job.id === d.jobId && !job.running) d.stage = "finished";
  const body = $("dialog-body");

  if (d.stage === "confirm") {
    if (body.dataset.stage === "confirm") return; // keep the user's toggles while polling
    const risky = d.items.filter((r) => r.verdict !== "safe");
    const bytes = d.items.reduce((s, r) => s + r.bytesFreed, 0);
    body.innerHTML = `
      <h2>Delete ${plural(d.items.length, "worktree", "worktrees")}?</h2>
      <p>Frees about ${formatBytes(bytes)}. Folders are deleted permanently (not moved to the Recycle Bin or Trash) and git's
      worktree records are cleaned up. Branches and commits stay in the repository.</p>
      <div class="dlist">${d.items.map((r) => `
        <div><span class="verdict verdict-${r.verdict}">${verdictIcon[r.verdict]}</span>
          <span>${esc(r.name)}<small>${esc(r.repoName)} · ${esc(r.branchLabel)}</small></span>
          <span class="badges">${r.badges.map((b) => `<span class="badge tone-${b.tone}">${esc(b.text)}</span>`).join("")}</span>
          <span class="num">${formatBytes(r.bytesFreed)}</span></div>`).join("")}
      </div>
      ${risky.length ? `<div class="warning">⚠ ${risky.length} of these have uncommitted changes, unpushed commits, are in use, or couldn't be checked.
        <label><input type="checkbox" id="ack"> I understand that work in these worktrees will be lost</label></div>` : ""}
      <label class="option"><input type="checkbox" id="branches"> Also delete their local branches when fully merged
        <small>Uses <code>git branch -d</code>, which keeps any branch with unmerged commits.</small></label>
      <div class="actions">
        <button id="d-cancel">Cancel</button>
        <button id="d-confirm" class="danger" ${risky.length ? "disabled" : ""}>Delete ${plural(d.items.length, "Worktree", "Worktrees")}</button>
      </div>`;
    body.dataset.stage = "confirm";
    const ack = $("ack");
    if (ack) ack.addEventListener("change", () => { $("d-confirm").disabled = !ack.checked; });
    $("d-cancel").addEventListener("click", closeDialog);
    $("d-confirm").addEventListener("click", () => startDelete($("branches").checked));
    return;
  }

  if (d.stage === "running") {
    const done = job && job.id === d.jobId ? job.done : 0;
    body.dataset.stage = "running";
    body.innerHTML = `<h2>Deleting worktrees…</h2>
      <progress max="${d.items.length}" value="${done}"></progress>
      <p>${done} of ${d.items.length} done</p>`;
    return;
  }

  if (body.dataset.stage === "finished") return;
  body.dataset.stage = "finished";
  const r = job.report;
  let freed = `Estimated ${formatBytes(r.estimatedFreed)} freed`;
  if (r.measuredFreed > 0) freed += ` · disk shows ${formatBytes(r.measuredFreed)} more free space`;
  body.innerHTML = `
    <h2>${r.failures.length ? "⚠" : "✔"} Deleted ${plural(r.removed, "worktree", "worktrees")}</h2>
    <p>${esc(freed)}.</p>
    ${r.branchesDeleted || r.branchesKept.length ? `<p>Deleted ${plural(r.branchesDeleted, "merged branch", "merged branches")}.
      ${r.branchesKept.length ? `Kept ${r.branchesKept.length} with unmerged commits: ${esc(r.branchesKept.join(", "))}` : ""}</p>` : ""}
    ${r.failures.length ? `<h3>Couldn't delete ${r.failures.length}:</h3><div class="failures">${r.failures.map((f) =>
      `<div>${esc(f.path)}<code>${esc(f.message)}</code></div>`).join("")}</div>` : ""}
    <div class="actions"><button id="d-done" class="primary">Done</button></div>`;
  $("d-done").addEventListener("click", closeDialog);
  $("d-done").focus();
}

async function startDelete(deleteBranches) {
  const d = ui.dialog;
  try {
    const res = await api("delete", { paths: d.items.map((r) => r.path), deleteBranches });
    d.jobId = res.job;
    d.stage = "running";
    for (const r of d.items) ui.checked.delete(r.path);
    renderDialog();
    refresh();
  } catch (err) { toast(err.message); }
}

function closeDialog() {
  if (ui.dialog && ui.dialog.stage === "running") return;
  ui.dialog = null;
  $("dialog-body").dataset.stage = "";
  $("dialog").close();
  render();
}

$("dialog").addEventListener("cancel", (e) => {
  if (ui.dialog && ui.dialog.stage === "running") e.preventDefault();
  else { e.preventDefault(); closeDialog(); }
});

refresh();
