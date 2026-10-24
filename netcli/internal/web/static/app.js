"use strict";

// ---- helpers ---------------------------------------------------------------

const $ = (s, el = document) => el.querySelector(s);
const $$ = (s, el = document) => [...el.querySelectorAll(s)];

async function api(path, opts = {}) {
  const init = { method: opts.method || "GET", headers: { "X-Netcli": "1" }, credentials: "same-origin" };
  if (opts.body !== undefined) {
    init.headers["Content-Type"] = "application/json";
    init.body = JSON.stringify(opts.body);
  }
  const res = await fetch(path, init);
  let data = {};
  try { data = await res.json(); } catch (_) { /* empty body */ }
  if (res.status === 401 && path !== "/api/login") { showLogin(); throw new Error("login required"); }
  if (!res.ok) throw new Error(data.error || res.statusText);
  return data;
}

function toast(msg) {
  const t = $("#toast");
  t.textContent = msg;
  t.hidden = false;
  clearTimeout(toast.timer);
  toast.timer = setTimeout(() => (t.hidden = true), 4000);
}

function fmtRate(v, unit) {
  const u = unit === "pps" ? ["p/s", "Kp/s", "Mp/s", "Gp/s"] : ["b/s", "Kb/s", "Mb/s", "Gb/s"];
  let i = 0;
  while (Math.abs(v) >= 1000 && i < u.length - 1) { v /= 1000; i++; }
  return (v >= 100 || i === 0 ? v.toFixed(0) : v.toFixed(1)) + " " + u[i];
}

function fmtBytes(v) {
  const u = ["B", "KB", "MB", "GB", "TB"];
  let i = 0;
  while (v >= 1024 && i < u.length - 1) { v /= 1024; i++; }
  return (i === 0 ? v.toFixed(0) : v.toFixed(1)) + " " + u[i];
}

function fmtTime(ms) {
  return new Date(ms).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit", second: "2-digit" });
}

function cssVar(name) { return getComputedStyle(document.documentElement).getPropertyValue(name).trim(); }

function el(tag, attrs = {}, ...children) {
  const e = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) {
    if (k === "class") e.className = v; else if (k === "text") e.textContent = v; else e.setAttribute(k, v);
  }
  for (const c of children) if (c != null) e.append(c);
  return e;
}

// ---- line chart --------------------------------------------------------------
// A small canvas time-series chart: one y axis, recessive grid, 2px lines,
// crosshair tooltip on hover.

class LineChart {
  constructor(container, series, format, opts = {}) {
    this.container = container;
    this.series = series; // [{label, color: "--series-1"}]
    this.format = format;
    this.fixedMax = opts.max;
    this.windowMs = opts.windowMs || 10 * 60 * 1000;
    this.data = []; // [{t, v: [..]}]
    this.hover = null;
    this.canvas = el("canvas", { role: "img", "aria-label": opts.label || "chart" });
    container.append(this.canvas);
    this.ro = new ResizeObserver(() => this.draw());
    this.ro.observe(container);
    this.canvas.addEventListener("mousemove", (e) => this.onMove(e));
    this.canvas.addEventListener("mouseleave", () => { this.hover = null; $("#tooltip").hidden = true; this.draw(); });
  }

  setData(data) { this.data = data; this.draw(); }

  geometry() {
    const r = this.container.getBoundingClientRect();
    return { w: r.width, h: r.height, left: 64, right: 10, top: 8, bottom: 22 };
  }

  scale() {
    const g = this.geometry();
    const now = this.data.length ? this.data[this.data.length - 1].t : Date.now();
    const t0 = now - this.windowMs;
    let max = this.fixedMax || 0;
    if (!this.fixedMax) for (const p of this.data) for (const v of p.v) max = Math.max(max, v);
    const nice = niceMax(max || 1);
    const pw = g.w - g.left - g.right, ph = g.h - g.top - g.bottom;
    return {
      g, t0, now, max: nice.max, ticks: nice.ticks,
      x: (t) => g.left + ((t - t0) / this.windowMs) * pw,
      y: (v) => g.top + ph - (v / nice.max) * ph,
    };
  }

  draw() {
    const { g, t0, now, max, ticks, x, y } = this.scale();
    if (g.w === 0) return;
    const dpr = window.devicePixelRatio || 1;
    const c = this.canvas;
    c.width = Math.round(g.w * dpr);
    c.height = Math.round(g.h * dpr);
    const ctx = c.getContext("2d");
    ctx.setTransform(dpr, 0, 0, dpr, 0, 0);
    ctx.clearRect(0, 0, g.w, g.h);
    ctx.font = "11px system-ui, sans-serif";

    // grid and y labels
    ctx.strokeStyle = cssVar("--grid");
    ctx.fillStyle = cssVar("--muted");
    ctx.lineWidth = 1;
    ctx.textAlign = "right";
    ctx.textBaseline = "middle";
    for (const v of ticks) {
      const yy = Math.round(y(v)) + 0.5;
      ctx.beginPath(); ctx.moveTo(g.left, yy); ctx.lineTo(g.w - g.right, yy); ctx.stroke();
      ctx.fillText(this.format(v), g.left - 8, yy);
    }
    // x labels every 2 minutes
    ctx.textAlign = "center";
    ctx.textBaseline = "top";
    const step = 2 * 60 * 1000;
    for (let t = Math.ceil(t0 / step) * step; t <= now; t += step) {
      ctx.fillText(new Date(t).toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" }), x(t), g.h - g.bottom + 6);
    }

    // series
    const pts = this.data.filter((p) => p.t >= t0);
    ctx.lineWidth = 2;
    ctx.lineJoin = "round";
    ctx.lineCap = "round";
    this.series.forEach((s, i) => {
      ctx.strokeStyle = cssVar(s.color);
      ctx.beginPath();
      let started = false, prevT = 0;
      for (const p of pts) {
        // break the line on gaps (missed samples)
        if (started && p.t - prevT > 3 * (this.interval || 2000)) started = false;
        const px = x(p.t), py = y(Math.min(p.v[i], max));
        if (started) ctx.lineTo(px, py); else { ctx.moveTo(px, py); started = true; }
        prevT = p.t;
      }
      ctx.stroke();
    });

    // crosshair
    if (this.hover) {
      const p = this.hover;
      const px = Math.round(x(p.t)) + 0.5;
      ctx.strokeStyle = cssVar("--muted");
      ctx.lineWidth = 1;
      ctx.beginPath(); ctx.moveTo(px, g.top); ctx.lineTo(px, g.h - g.bottom); ctx.stroke();
      this.series.forEach((s, i) => {
        ctx.fillStyle = cssVar(s.color);
        ctx.strokeStyle = cssVar("--surface");
        ctx.lineWidth = 2;
        ctx.beginPath(); ctx.arc(px, y(Math.min(p.v[i], max)), 4, 0, 2 * Math.PI); ctx.fill(); ctx.stroke();
      });
    }
  }

  onMove(e) {
    if (!this.data.length) return;
    const { x } = this.scale();
    const r = this.canvas.getBoundingClientRect();
    const mx = e.clientX - r.left;
    let best = null, bd = Infinity;
    for (const p of this.data) {
      const d = Math.abs(x(p.t) - mx);
      if (d < bd) { bd = d; best = p; }
    }
    if (!best || bd > 40) { this.hover = null; $("#tooltip").hidden = true; this.draw(); return; }
    this.hover = best;
    this.draw();
    const tip = $("#tooltip");
    tip.replaceChildren(el("div", { class: "t", text: fmtTime(best.t) }),
      ...this.series.map((s, i) => {
        const dot = el("i"); dot.style.background = cssVar(s.color);
        return el("div", { class: "row" }, dot, `${s.label}: ${this.format(best.v[i])}`);
      }));
    tip.hidden = false;
    const tw = tip.offsetWidth;
    let left = e.clientX + 14;
    if (left + tw > window.innerWidth - 8) left = e.clientX - tw - 14;
    tip.style.left = left + "px";
    tip.style.top = e.clientY + 14 + "px";
  }
}

// Round the axis to a tick step of 1, 2, 2.5 or 5 x 10^n so labels stay short.
function niceMax(max) {
  const raw = max / 4;
  const exp = Math.pow(10, Math.floor(Math.log10(raw)));
  const f = raw / exp;
  const step = (f <= 1 ? 1 : f <= 2 ? 2 : f <= 2.5 ? 2.5 : f <= 5 ? 5 : 10) * exp;
  const top = Math.ceil(max / step) * step;
  const ticks = [];
  for (let v = 0; v <= top + step / 2; v += step) ticks.push(v);
  return { max: top, ticks };
}

function legend(target, series) {
  target.replaceChildren(...series.map((s) => {
    const i = el("i"); i.style.background = cssVar(s.color);
    return el("span", {}, i, s.label);
  }));
}

// ---- state --------------------------------------------------------------------

const state = {
  unit: "bps",
  showIdle: false,
  points: [],
  status: null,
  charts: new Map(), // iface -> {chart, card, now}
  sysChart: null,
  es: null,
};

const RATE_SERIES = [{ label: "In", color: "--series-1" }, { label: "Out", color: "--series-2" }];

function ifaceSeries(name) {
  const k = state.unit === "pps" ? ["rx_pps", "tx_pps"] : ["rx_bps", "tx_bps"];
  return state.points.filter((p) => p.ifaces && p.ifaces[name]).map((p) => ({ t: p.t, v: [p.ifaces[name][k[0]], p.ifaces[name][k[1]]] }));
}

function isIdle(name) {
  return !state.points.some((p) => p.ifaces && p.ifaces[name] && (p.ifaces[name].rx_bps > 0 || p.ifaces[name].tx_bps > 0));
}

function chartableInterfaces() {
  const names = new Set();
  for (const p of state.points) for (const n of Object.keys(p.ifaces || {})) names.add(n);
  const order = (state.status ? state.status.interfaces.map((i) => i.name) : []);
  return [...names]
    .filter((n) => !/^(lo|npflog|ifb|veth|docker)/.test(n))
    .filter((n) => state.showIdle || !isIdle(n))
    .sort((a, b) => {
      const ia = order.indexOf(a), ib = order.indexOf(b);
      return (ia < 0 ? 999 : ia) - (ib < 0 ? 999 : ib) || a.localeCompare(b);
    });
}

function renderDashboard() {
  const last = state.points[state.points.length - 1];
  if (last) {
    $("#tile-cpu").textContent = last.cpu.toFixed(1) + " %";
    $("#tile-mem").textContent = last.mem ? last.mem.toFixed(1) + " %" : "–";
    let rin = 0, rout = 0;
    for (const [n, r] of Object.entries(last.ifaces || {})) {
      if (/^(lo|bridge|vlan|wlan|npflog)/.test(n)) continue; // count physical links once
      rin += state.unit === "pps" ? r.rx_pps : r.rx_bps;
      rout += state.unit === "pps" ? r.tx_pps : r.tx_bps;
    }
    $("#tile-in").textContent = fmtRate(rin, state.unit);
    $("#tile-out").textContent = fmtRate(rout, state.unit);
  }
  if (!state.sysChart) {
    const sys = [{ label: "CPU", color: "--series-1" }, { label: "Memory", color: "--series-2" }];
    state.sysChart = new LineChart($("#chart-sys"), sys, (v) => v.toFixed(0) + " %", { max: 100, label: "CPU and memory usage" });
    legend($("#legend-sys"), sys);
  }
  state.sysChart.setData(state.points.map((p) => ({ t: p.t, v: [p.cpu, p.mem] })));

  const wanted = chartableInterfaces();
  for (const [name, c] of state.charts) {
    if (!wanted.includes(name)) { c.card.remove(); state.charts.delete(name); }
  }
  const grid = $("#if-charts");
  wanted.forEach((name, idx) => {
    let c = state.charts.get(name);
    if (!c) {
      const now = el("span", { class: "now" });
      const leg = el("div", { class: "legend" });
      const box = el("div", { class: "chart" });
      const st = state.status && state.status.interfaces.find((i) => i.name === name);
      const title = el("h2", { text: name + (st && st.description ? " · " + st.description : "") });
      const card = el("div", { class: "card" }, el("div", { class: "card-head" }, title, leg, now), box);
      legend(leg, RATE_SERIES);
      c = { card, now, chart: new LineChart(box, RATE_SERIES, (v) => fmtRate(v, state.unit), { label: "Traffic on " + name }) };
      state.charts.set(name, c);
    }
    if (grid.children[idx] !== c.card) grid.insertBefore(c.card, grid.children[idx] || null);
    const data = ifaceSeries(name);
    c.chart.interval = (state.status && state.status.interval * 1000) || 2000;
    c.chart.format = (v) => fmtRate(v, state.unit);
    c.chart.setData(data);
    const l = data[data.length - 1];
    c.now.textContent = l ? `in ${fmtRate(l.v[0], state.unit)} · out ${fmtRate(l.v[1], state.unit)}` : "";
  });
  if (!wanted.length) grid.replaceChildren(el("p", { class: "muted", text: "No interface traffic yet. Tick “Show idle interfaces” to see all." }));
  else $$("p.muted", grid).forEach((p) => p.remove());
}

function startStream() {
  if (state.es) state.es.close();
  const es = new EventSource("/api/stats/stream");
  state.es = es;
  es.onopen = () => ($("#stream-state").textContent = "live");
  es.onerror = () => ($("#stream-state").textContent = "reconnecting…");
  es.onmessage = (ev) => {
    const p = JSON.parse(ev.data);
    state.points.push(p);
    const cutoff = p.t - 10 * 60 * 1000;
    while (state.points.length && state.points[0].t < cutoff) state.points.shift();
    if (!$("#tab-dashboard").hidden) renderDashboard();
  };
}

// ---- status & interfaces --------------------------------------------------------

async function refreshStatus() {
  const s = await api("/api/status");
  state.status = s;
  $("#hostname").textContent = s.hostname || "router";
  document.title = (s.hostname || "router") + " · netcli";
  $("#platform").textContent = `netcli ${s.version} · ${s.platform}`;
  $("#badge-dry").hidden = !s.dry_run;
  $("#badge-unsaved").hidden = !s.unsaved;
  $("#btn-save").hidden = !s.unsaved;
  const rb = $("#badge-revert");
  if (s.revert_at) {
    const left = Math.max(0, Math.round((s.revert_at - Date.now()) / 1000));
    rb.textContent = `rollback in ${Math.floor(left / 60)}:${String(left % 60).padStart(2, "0")}`;
    rb.title = "Confirm with 'configure confirm' in the terminal";
    rb.hidden = false;
  } else rb.hidden = true;
  $("#tile-ifs").textContent = `${s.interfaces.filter((i) => i.up && i.running && !/^lo/.test(i.name)).length} / ${s.interfaces.filter((i) => !/^lo/.test(i.name)).length}`;
  renderInterfaces();
}

function renderInterfaces() {
  const tb = $("#if-table tbody");
  tb.replaceChildren(...state.status.interfaces.map((i) => {
    let cls = "down", txt = "down";
    if (i.shutdown) txt = "admin down";
    else if (!i.present) txt = "not present";
    else if (i.up && i.running) { cls = "up"; txt = "up"; }
    else if (i.up) txt = "no carrier";
    const filters = [i.acl_in && "in: " + i.acl_in, i.acl_out && "out: " + i.acl_out, i.wireless && "wifi: " + i.wireless].filter(Boolean).join("\n");
    return el("tr", {},
      el("td", {}, el("strong", { text: i.name }), i.configured ? null : el("div", { class: "muted", text: "not configured" })),
      el("td", {}, el("span", { class: "pill " + cls, text: txt })),
      el("td", { text: i.description || "" }),
      el("td", { text: (i.addresses.join("\n") || (i.dhcp ? "DHCP (no lease)" : "")) , style: "white-space:pre" }),
      el("td", { text: [i.mac, i.mtu ? "MTU " + i.mtu : ""].filter(Boolean).join("\n"), style: "white-space:pre" }),
      el("td", { class: "num", text: fmtBytes(i.rx_bytes) + (i.rx_errs ? `\n${i.rx_errs} err` : ""), style: "white-space:pre" }),
      el("td", { class: "num", text: fmtBytes(i.tx_bytes) + (i.tx_errs ? `\n${i.tx_errs} err` : ""), style: "white-space:pre" }),
      el("td", { text: filters, style: "white-space:pre" }));
  }));
}

// ---- configuration ----------------------------------------------------------------

let yamlDirty = false;

async function loadConfig(force) {
  const run = await api("/api/config?which=running");
  $("#cfg-cli").textContent = run.cli;
  if (force || !yamlDirty) { $("#cfg-yaml").value = run.yaml; yamlDirty = false; }
  const st = await api("/api/config?which=startup");
  $("#cfg-diff").textContent = st.present ? st.diff || "No differences: the running configuration is saved." : "There is no startup configuration yet.";
}

function showResult(text, ok) {
  const r = $("#cfg-result");
  r.textContent = text;
  r.className = "code result " + (ok ? "ok" : "bad");
  r.hidden = false;
}

async function validateYaml() {
  const res = await api("/api/config/validate", { method: "POST", body: { yaml: $("#cfg-yaml").value } });
  if (!res.ok) showResult(res.error, false);
  else showResult(res.diff ? "Valid. Changes compared with the running configuration:\n\n" + res.diff : "Valid. No changes.", true);
  return res.ok;
}

async function applyYaml() {
  try {
    await api("/api/config", { method: "PUT", body: { yaml: $("#cfg-yaml").value } });
    yamlDirty = false;
    showResult("Applied. Remember to save to the startup configuration.", true);
    await Promise.all([loadConfig(true), refreshStatus()]);
  } catch (e) { showResult(e.message, false); }
}

// ---- terminal ------------------------------------------------------------------------

const term = { history: [], pos: 0 };

function termWrite(text) {
  const out = $("#term-out");
  out.textContent += text;
  if (out.textContent.length > 200000) out.textContent = out.textContent.slice(-150000);
  out.scrollTop = out.scrollHeight;
}

async function termRun(line) {
  if (line.trim()) { term.history.push(line); }
  term.pos = term.history.length;
  const res = await api("/api/cli", { method: "POST", body: { line } });
  termWrite(res.output);
  $("#term-prompt").textContent = res.prompt;
  if (/^\s*(conf|wr|copy|no |int|ip |end|exit)/.test(line) || !$("#tab-dashboard").hidden) refreshStatus().catch(() => {});
}

function setupTerminal() {
  const input = $("#term-line");
  input.addEventListener("keydown", async (e) => {
    if (e.key === "Enter") {
      e.preventDefault();
      const line = input.value;
      input.value = "";
      try { await termRun(line); } catch (err) { termWrite("% " + err.message + "\n"); }
    } else if (e.key === "?" && input.selectionStart === input.value.length && (input.value.split('"').length % 2 === 1)) {
      e.preventDefault();
      const res = await api("/api/cli/help", { method: "POST", body: { line: input.value } });
      termWrite(res.prompt + input.value + "?\n" + res.output);
    } else if (e.key === "Tab") {
      e.preventDefault();
      const v = input.value;
      const res = await api("/api/cli/complete", { method: "POST", body: { line: v } });
      const opts = res.options || [];
      const start = /\s$/.test(v) || v === "" ? v.length : v.search(/\S+$/);
      if (opts.length === 1) input.value = v.slice(0, start) + opts[0] + " ";
      else if (opts.length > 1) {
        let cp = opts[0];
        for (const o of opts) while (!o.toLowerCase().startsWith(cp.toLowerCase())) cp = cp.slice(0, -1);
        if (cp.length > v.length - start) input.value = v.slice(0, start) + cp;
        else termWrite($("#term-prompt").textContent + v + "\n" + opts.join("  ") + "\n");
      }
    } else if (e.key === "ArrowUp") {
      e.preventDefault();
      if (term.pos > 0) input.value = term.history[--term.pos];
    } else if (e.key === "ArrowDown") {
      e.preventDefault();
      term.pos = Math.min(term.history.length, term.pos + 1);
      input.value = term.history[term.pos] || "";
    } else if (e.key === "z" && e.ctrlKey) {
      e.preventDefault();
      input.value = "";
      termRun("end").catch(() => {});
    }
  });
  $("#term-out").addEventListener("click", () => input.focus());
}

// ---- wiring ------------------------------------------------------------------------------

function showTab(name) {
  $$(".tabs button").forEach((b) => b.setAttribute("aria-selected", String(b.dataset.tab === name)));
  $$(".tab").forEach((t) => (t.hidden = t.id !== "tab-" + name));
  if (name === "config") loadConfig().catch((e) => toast(e.message));
  if (name === "interfaces") refreshStatus().catch(() => {});
  if (name === "dashboard") { state.charts.forEach((c) => c.chart.draw()); renderDashboard(); }
  if (name === "terminal") {
    $("#term-line").focus();
    if (!$("#term-prompt").textContent) termRun("").catch(() => {});
  }
}

function showLogin() {
  $("#app").hidden = true;
  $("#login").hidden = false;
  if (state.es) { state.es.close(); state.es = null; }
  $("#login-user").focus();
}

async function startApp() {
  $("#login").hidden = true;
  $("#app").hidden = false;
  await refreshStatus();
  state.points = await api("/api/stats/history");
  renderDashboard();
  startStream();
  clearInterval(startApp.timer);
  startApp.timer = setInterval(() => refreshStatus().catch(() => {}), 10000);
}

function setup() {
  $("#login-form").addEventListener("submit", async (e) => {
    e.preventDefault();
    $("#login-error").textContent = "";
    try {
      await api("/api/login", { method: "POST", body: { username: $("#login-user").value, password: $("#login-pass").value } });
      $("#login-pass").value = "";
      await startApp();
    } catch (err) { $("#login-error").textContent = err.message; }
  });
  $("#btn-logout").addEventListener("click", async () => { await api("/api/logout", { method: "POST" }).catch(() => {}); showLogin(); });
  $("#btn-save").addEventListener("click", async () => {
    try { await api("/api/config/save", { method: "POST" }); toast("Saved to the startup configuration"); refreshStatus(); }
    catch (e) { toast(e.message); }
  });
  $$(".tabs button").forEach((b) => b.addEventListener("click", () => showTab(b.dataset.tab)));
  $$("[data-unit]").forEach((b) => b.addEventListener("click", () => {
    state.unit = b.dataset.unit;
    $$("[data-unit]").forEach((x) => x.setAttribute("aria-pressed", String(x === b)));
    renderDashboard();
  }));
  $("#show-idle").addEventListener("change", (e) => { state.showIdle = e.target.checked; renderDashboard(); });
  $$("[data-view]").forEach((b) => b.addEventListener("click", () => {
    $$("[data-view]").forEach((x) => x.setAttribute("aria-pressed", String(x === b)));
    for (const v of ["cli", "yaml", "diff"]) $("#view-" + v).hidden = v !== b.dataset.view;
    loadConfig().catch((e) => toast(e.message));
  }));
  $("#cfg-yaml").addEventListener("input", () => (yamlDirty = true));
  $("#btn-validate").addEventListener("click", () => validateYaml().catch((e) => showResult(e.message, false)));
  $("#btn-apply").addEventListener("click", async () => { if (await validateYaml().catch(() => false)) applyYaml(); });
  $("#btn-reload").addEventListener("click", () => { $("#cfg-result").hidden = true; loadConfig(true); });
  setupTerminal();
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
    state.sysChart && state.sysChart.draw();
    state.charts.forEach((c) => c.chart.draw());
    legend($("#legend-sys"), [{ label: "CPU", color: "--series-1" }, { label: "Memory", color: "--series-2" }]);
    $$("#if-charts .legend").forEach((l) => legend(l, RATE_SERIES));
  });

  api("/api/session").then(startApp).catch(() => showLogin());
}

setup();
