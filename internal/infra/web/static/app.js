// didban status page: polls api/status and api/history and draws them.
// No libraries, so the page loads where no CDN is reachable.
"use strict";

const $ = (id) => document.getElementById(id);
const SVG = "http://www.w3.org/2000/svg";

const HARDWARE = [
  ["bmp180", "BMP180", "0x77", "temperature and pressure"],
  ["ds3231", "DS3231", "0x68", "real-time clock"],
  ["pcf8591", "PCF8591", "0x48", "4-channel ADC"],
  ["buzzer", "PCF8574", "0x20", "buzzer (bit 7, active-low)"],
  ["joystick", "Joystick", "0x20 + GPIO 20", "5-way switch: directions on PCF8574 bits 0–3, centre on BCM 20"],
  ["ds18b20", "DS18B20", "1-Wire", "temperature probe (optional)"],
  ["lcd", "LCD", "", "character display (optional)"],
];

let zone = "Asia/Tehran";
let pollMs = 15000;
let last = null; // last good status
let lastFetch = 0;
let failing = false;

function el(tag, attrs = {}, ...children) {
  const n = document.createElement(tag);
  for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v);
  for (const c of children) n.append(c);
  return n;
}

function svg(tag, attrs = {}) {
  const n = document.createElementNS(SVG, tag);
  for (const [k, v] of Object.entries(attrs)) n.setAttribute(k, v);
  return n;
}

function fmtTime(d, withSeconds) {
  return new Intl.DateTimeFormat("en-GB", {
    timeZone: zone, hour: "2-digit", minute: "2-digit",
    ...(withSeconds ? { second: "2-digit" } : {}),
  }).format(d);
}

function fmtDate(d) {
  return new Intl.DateTimeFormat("en-CA", { timeZone: zone, year: "numeric", month: "2-digit", day: "2-digit" }).format(d);
}

function fmtDuration(sec) {
  sec = Math.abs(Math.round(sec));
  const d = Math.floor(sec / 86400), h = Math.floor(sec % 86400 / 3600), m = Math.floor(sec % 3600 / 60), s = sec % 60;
  if (d) return `${d} d ${h} h`;
  if (h) return `${h} h ${m} m`;
  if (m) return `${m} m ${s} s`;
  return `${s} s`;
}

function setValue(id, text, unit) {
  const n = $(id);
  n.textContent = text;
  if (unit) n.append(el("small", {}, unit));
}

async function getJSON(path) {
  const res = await fetch(path, { cache: "no-store" });
  if (!res.ok) throw new Error(`${path}: HTTP ${res.status}`);
  return res.json();
}

// shortVersion keeps a tag as is and turns an untagged build's pseudo-version
// into "rev 35bcfdb" (plus "dirty" for a build from a modified tree).
function shortVersion(v) {
  if (!v || !v.startsWith("v0.0.0-")) return v || "";
  const rev = v.match(/rev-([0-9a-f]{7})/);
  return `rev ${rev ? rev[1] : "unknown"}${v.endsWith("dirty") ? " (dirty)" : ""}`;
}

// --- status ---------------------------------------------------------------

function renderStatus(s) {
  zone = s.timezone || zone;
  pollMs = Math.max(5, s.poll_seconds || 15) * 1000;
  document.title = `didban · ${s.host}`;
  $("host").textContent = s.host;
  $("today").textContent = `${new Intl.DateTimeFormat("en-GB", { timeZone: zone, weekday: "long", day: "numeric", month: "long" }).format(new Date(s.now))} · readings every ${s.poll_seconds} s from the sensor HAT on ${s.host}.`;

  const r = s.reading || {};

  // A DS18B20 probe is the room; the BMP180 sits over the Pi's CPU and reads
  // the board's heat, so it is the fallback.
  const probe = (r.probes || [])[0];
  if (probe) {
    setValue("temp", probe.celsius.toFixed(1), "°C");
    $("temp-sub").textContent = `DS18B20 probe${r.temperature_c != null ? ` · board ${r.temperature_c.toFixed(1)} °C (BMP180)` : ""}`;
  } else {
    setValue("temp", r.temperature_c != null ? r.temperature_c.toFixed(1) : "–", r.temperature_c != null ? "°C" : "");
    $("temp-sub").textContent = r.rtc_temperature_c != null
      ? `BMP180, board heat · RTC die ${r.rtc_temperature_c.toFixed(2)} °C`
      : "BMP180, board heat";
  }
  renderProbes(r.probes || []);
  if (s.joystick) renderJoystick(s.joystick);
  $("joystick-panel").hidden = !(s.sensors || {}).joystick;

  if (r.pressure_pa != null) setValue("pressure", (r.pressure_pa / 100).toFixed(1), "hPa");
  else setValue("pressure", "–");

  renderClock(r, s.rtc_drift_seconds);

  setValue("uptime", fmtDuration((new Date(s.now) - new Date(s.started)) / 1000));
  $("uptime-sub").textContent = `up since ${fmtDate(new Date(s.started))} ${fmtTime(new Date(s.started))} · ${shortVersion(s.version)}`;

  renderADC(r.adc || []);
  renderHardware(s.sensors || {});
}

function renderClock(r, drift) {
  const sub = $("clock-sub");
  sub.className = "sub";
  if (!r.rtc_time) {
    setValue("clock", "–");
    sub.textContent = "no RTC reading";
    return;
  }
  const t = new Date(r.rtc_time);
  setValue("clock", fmtTime(t, true));
  if (drift == null) {
    sub.textContent = "real-time clock";
  } else if (Math.abs(drift) < 2) {
    sub.textContent = "in step with the Pi's clock";
    sub.classList.add("green");
  } else {
    const date = fmtDate(t) !== fmtDate(new Date(r.at)) ? ` (holds ${fmtDate(t)})` : "";
    sub.textContent = `${fmtDuration(drift)} ${drift > 0 ? "ahead of" : "behind"} the Pi${date}`;
    sub.classList.add("orange");
  }
}

function renderADC(channels) {
  const ul = $("adc");
  if (!channels.length) {
    ul.replaceChildren(el("li", { class: "empty" }, "No ADC reading."));
    return;
  }
  ul.replaceChildren(...channels.map((c) => {
    const bar = el("span");
    bar.style.width = `${Math.max(1, c.raw / 255 * 100).toFixed(1)}%`;
    return el("li", {},
      el("span", { class: "name" }, c.label || `ain${c.index}`),
      el("span", { class: "bar", role: "meter", "aria-valuemin": "0", "aria-valuemax": "255", "aria-valuenow": String(c.raw) }, bar),
      el("span", { class: "num" }, `${c.volts.toFixed(2)} V `, el("small", {}, `(${c.raw})`)));
  }));
}

function renderProbes(probes) {
  const ul = $("probes");
  if (!probes.length) {
    ul.replaceChildren(el("li", { class: "empty" }, "No DS18B20 on the 1-Wire bus."));
    return;
  }
  ul.replaceChildren(...probes.map((p) => el("li", {},
    el("span", { class: "grow" }, el("span", { class: "name" }, `${p.celsius.toFixed(2)} °C`), el("br"), el("code", {}, p.id)))));
}

let lastPress = null;

function renderJoystick(js) {
  const held = new Set(js.pressed || []);
  const newest = (js.recent || [])[0];
  const fresh = newest && newest.at !== lastPress;
  document.querySelectorAll(".key").forEach((k) => {
    const key = k.dataset.key;
    k.classList.toggle("held", held.has(key));
    k.querySelector("b").textContent = String((js.counts || {})[key] || 0);
    if (fresh && newest.key === key && !held.has(key)) {
      k.classList.add("flash");
      setTimeout(() => k.classList.remove("flash"), 600);
    }
  });
  if (newest) lastPress = newest.at;
  const recent = js.recent || [];
  $("joy-recent").replaceChildren(...recent.slice(0, 6).map((p) =>
    el("li", {}, el("span", { class: "grow" }, p.key), el("span", { class: "muted" }, `${fmtDate(new Date(p.at))} ${fmtTime(new Date(p.at), true)}`))));
  $("joy-note").textContent = recent.length
    ? "Lit while held; numbers are presses since the daemon started."
    : "Press the joystick on the board: the key lights here and the press is counted.";
}

// The joystick is polled fast, but only while the page is visible.
async function refreshJoystick() {
  if (!document.hidden && last && (last.sensors || {}).joystick) {
    try {
      renderJoystick(await getJSON("api/joystick"));
    } catch {
      // the status banner reports an unreachable daemon.
    }
  }
  setTimeout(refreshJoystick, 400);
}

function renderHardware(found) {
  $("hardware").replaceChildren(...HARDWARE.map(([key, chip, addr, what]) => {
    const ok = !!found[key];
    const optional = { lcd: "off", ds18b20: "not attached" };
    const state = ok
      ? el("span", { class: "chip ok" }, "found")
      : el("span", { class: key in optional ? "chip" : "chip bad" }, optional[key] || "missing");
    return el("li", {},
      el("span", { class: "grow" }, el("span", { class: "name" }, chip), " ", addr ? el("code", {}, addr) : "", el("br"), el("span", { class: "muted small" }, what)),
      state);
  }));
  document.querySelectorAll("[data-beep]").forEach((b) => {
    if (!found.buzzer) b.disabled = true;
  });
}

// --- liveness -------------------------------------------------------------

function renderLive() {
  const live = $("live");
  live.classList.remove("ok", "stale", "down");
  if (failing || !last) {
    live.classList.add("down");
    $("age").textContent = last ? "daemon not answering" : "connecting";
    return;
  }
  const at = last.reading ? new Date(last.reading.at) : null;
  if (!at) {
    live.classList.add("stale");
    $("age").textContent = "no reading yet";
    return;
  }
  // Age by the server's clock, plus the time since we asked it.
  const age = (new Date(last.now) - at) / 1000 + (Date.now() - lastFetch) / 1000;
  live.classList.add(age < pollMs / 1000 * 3 ? "ok" : "stale");
  $("age").textContent = `read ${fmtDuration(age)} ago`;
}

async function refreshStatus() {
  try {
    last = await getJSON("api/status");
    lastFetch = Date.now();
    failing = false;
    $("error").hidden = true;
    renderStatus(last);
  } catch (e) {
    failing = true;
    $("error").textContent = `Cannot reach the didban daemon: ${e.message}. The numbers below are the last ones it sent.`;
    $("error").hidden = false;
  }
  renderLive();
  setTimeout(refreshStatus, pollMs);
}

// --- history charts -------------------------------------------------------

function drawChart(fig, points, unit, color, digits) {
  if (points.length < 2) {
    fig.replaceChildren(el("p", { class: "empty" }, "Collecting… one point a minute."));
    return;
  }
  const W = 600, H = 190, L = 46, R = 10, T = 10, B = 24;
  const x0 = points[0].t, x1 = points[points.length - 1].t;
  let y0 = Math.min(...points.map((p) => p.v)), y1 = Math.max(...points.map((p) => p.v));
  const pad = Math.max((y1 - y0) * 0.15, digits ? 0.3 : 0.5);
  y0 -= pad; y1 += pad;
  const sx = (t) => L + (t - x0) / Math.max(1, x1 - x0) * (W - L - R);
  const sy = (v) => T + (1 - (v - y0) / (y1 - y0)) * (H - T - B);

  const root = svg("svg", { viewBox: `0 0 ${W} ${H}`, role: "img", "aria-label": `${unit} over the last ${fmtDuration((x1 - x0) / 1000)}` });
  for (let i = 0; i <= 3; i++) {
    const v = y0 + (y1 - y0) * i / 3, y = sy(v);
    root.append(svg("line", { class: "grid", x1: L, x2: W - R, y1: y, y2: y }));
    const label = svg("text", { class: "axis", x: L - 6, y: y + 4, "text-anchor": "end" });
    label.textContent = v.toFixed(digits);
    root.append(label);
  }
  [[x0, "start"], [(x0 + x1) / 2, "middle"], [x1, "end"]].forEach(([t, anchor]) => {
    const label = svg("text", { class: "axis", x: sx(t), y: H - 6, "text-anchor": anchor });
    label.textContent = fmtTime(new Date(t));
    root.append(label);
  });

  const line = points.map((p) => `${sx(p.t).toFixed(1)},${sy(p.v).toFixed(1)}`).join(" ");
  root.append(svg("polygon", { class: `area ${color}`, points: `${sx(x0)},${H - B} ${line} ${sx(x1)},${H - B}` }));
  root.append(svg("polyline", { class: `line ${color}`, points: line }));
  const cursor = svg("line", { class: "cursor", y1: T, y2: H - B, visibility: "hidden" });
  const dot = svg("circle", { class: `dot ${color}`, r: 4.5, visibility: "hidden" });
  root.append(cursor, dot);

  const min = points.reduce((a, p) => (p.v < a.v ? p : a)), max = points.reduce((a, p) => (p.v > a.v ? p : a));
  const left = el("span"), right = el("span");
  const idle = () => {
    left.replaceChildren(`min `, el("b", {}, `${min.v.toFixed(digits)} ${unit}`), ` at ${fmtTime(new Date(min.t))}`);
    right.replaceChildren(`max `, el("b", {}, `${max.v.toFixed(digits)} ${unit}`), ` at ${fmtTime(new Date(max.t))}`);
    cursor.setAttribute("visibility", "hidden");
    dot.setAttribute("visibility", "hidden");
  };
  root.addEventListener("pointermove", (ev) => {
    const box = root.getBoundingClientRect();
    const t = x0 + ((ev.clientX - box.left) / box.width * W - L) / (W - L - R) * (x1 - x0);
    const p = points.reduce((a, q) => (Math.abs(q.t - t) < Math.abs(a.t - t) ? q : a));
    cursor.setAttribute("x1", sx(p.t)); cursor.setAttribute("x2", sx(p.t));
    dot.setAttribute("cx", sx(p.t)); dot.setAttribute("cy", sy(p.v));
    cursor.setAttribute("visibility", "visible"); dot.setAttribute("visibility", "visible");
    left.replaceChildren(el("b", {}, `${p.v.toFixed(digits)} ${unit}`), ` at ${fmtTime(new Date(p.t))}`);
    right.replaceChildren("");
  });
  root.addEventListener("pointerleave", idle);
  idle();
  fig.replaceChildren(root, el("figcaption", {}, left, right));
}

function renderTrend(points) {
  const sub = $("pressure-sub");
  sub.className = "sub";
  if (points.length < 2 || points[points.length - 1].t - points[0].t < 3600e3) {
    sub.textContent = "trend after an hour of data";
    return;
  }
  const now = points[points.length - 1];
  const target = now.t - 3 * 3600e3;
  const then = points.reduce((a, p) => (Math.abs(p.t - target) < Math.abs(a.t - target) ? p : a));
  const delta = now.v - then.v;
  const span = fmtDuration((now.t - then.t) / 1000);
  if (Math.abs(delta) < 0.5) {
    sub.textContent = `steady over ${span}`;
  } else {
    sub.textContent = `${delta > 0 ? "▲ rising" : "▼ falling"} ${Math.abs(delta).toFixed(1)} hPa in ${span}`;
    sub.classList.add(delta > 0 ? "green" : "orange");
  }
}

async function refreshHistory() {
  try {
    const samples = await getJSON("api/history");
    const series = (key, scale) => samples
      .filter((s) => s[key] != null)
      .map((s) => ({ t: new Date(s.t).getTime(), v: s[key] * scale }));
    drawChart($("chart-temp"), series("temperature_c", 1), "°C", "pink", 1);
    const pressure = series("pressure_pa", 0.01);
    drawChart($("chart-pressure"), pressure, "hPa", "aqua", 1);
    renderTrend(pressure);
  } catch {
    // the status banner already says the daemon is unreachable.
  }
  setTimeout(refreshHistory, 60e3);
}

// --- buzzer ---------------------------------------------------------------

async function beep(count, buttons) {
  buttons.forEach((b) => { b.disabled = true; });
  const state = $("beep-state");
  state.textContent = "beeping…";
  try {
    const res = await fetch("api/beep", { method: "POST", headers: { "Content-Type": "application/json" }, body: JSON.stringify({ count }) });
    state.textContent = res.ok ? "done" : (await res.text()).trim();
  } catch (e) {
    state.textContent = e.message;
  }
  buttons.forEach((b) => { b.disabled = false; });
  setTimeout(() => { state.textContent = ""; }, 3000);
}

document.addEventListener("DOMContentLoaded", () => {
  const buttons = [...document.querySelectorAll("[data-beep]")];
  buttons.forEach((b) => b.addEventListener("click", () => beep(Number(b.dataset.beep), buttons)));
  refreshStatus();
  refreshHistory();
  refreshJoystick();
  setInterval(renderLive, 1000);
});
