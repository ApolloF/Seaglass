// Frame times and memory of the real app with a big library on a 4K
// screen (3840×2160 at 150 %): each scenario holds a controller direction
// for a few seconds, while the page records every frame and long task.
//
//   node perf.mjs [--games=500,2000] [--only=deck-row,orbit] [--seconds=5] [--frames]
//
// --frames also records a screencast of each scenario (to look for
// flicker). Results: %TEMP%\wl-harness\perf\results.json and a table.
import { execFileSync } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import { OUT, backupAppData, makeDevData, restoreAppData, sleep, startApp } from "./lib.mjs";

const argv = process.argv.slice(2);
const opt = (k, d) => argv.find((a) => a.startsWith(`--${k}=`))?.split("=")[1] ?? d;
const counts = opt("games", "500,2000").split(",").map(Number);
const only = opt("only", "");
const seconds = Number(opt("seconds", 5));
const dir = path.join(OUT, "perf");
fs.mkdirSync(dir, { recursive: true });

// Each scenario: how to get there (from big picture's Home), then what to hold.
const SCENARIOS = [
  { name: "desktop-grid", desktop: true, hold: ["down"] },
  { name: "deck-row", layout: "deck", setup: ["down"], hold: ["right"] },
  { name: "deck-library", layout: "deck", setup: ["rb"], hold: ["down"] },
  { name: "console-row", layout: "console", hold: ["right"] },
  { name: "orbit", layout: "orbit", hold: ["right", "down"] },
  { name: "search", layout: "deck", setup: ["rb", "rb", "south"], hold: ["right"] },
];

const results = [];
backupAppData();
try {
  for (const n of counts) {
    for (const layout of ["desktop", "deck", "console", "orbit"]) {
      const scen = SCENARIOS.filter((s) => (s.desktop ? layout === "desktop" : s.layout === layout) && (!only || only.split(",").includes(s.name)));
      if (!scen.length) continue;
      const data = makeDevData(`perf-${n}`, { extra: n, settings: { bigPictureLayout: layout === "desktop" ? "deck" : layout, startInBigPicture: layout !== "desktop" } });
      const app = await startApp({ data });
      try {
        await app.viewport(3840, 2160, 1.5);
        await sleep(3000); // art loads, the page settles
        for (const s of scen) results.push(await run(app, n, s));
      } finally {
        await app.quit();
      }
    }
  }
} finally {
  restoreAppData();
}
fs.writeFileSync(path.join(dir, "results.json"), JSON.stringify(results, null, 1));
console.log("\ngames scenario         fps  presented partial dropped holes  main ms  longtasks  heapMB   DOM  webview MB  core MB");
for (const r of results) {
  const c = (v, w) => String(v).padStart(w);
  console.log(
    `${c(r.games, 5)} ${r.name.padEnd(14)} ${c(r.fps.toFixed(0), 4)} ${c(r.presented, 10)} ${c(r.partial, 7)} ${c(r.dropped, 7)} ${c(r.holes, 5)} ${c(r.mainMs.toFixed(0), 8)} ${c(Math.round(r.longTaskMs), 10)} ${c(r.heapMB.toFixed(0), 7)} ${c(r.dom, 5)} ${c(r.webviewMB.toFixed(0), 11)} ${c(r.coreMB.toFixed(0), 8)}`,
  );
}

async function run(app, games, s) {
  for (const b of s.setup ?? []) await app.pad.press(b, 600);
  await sleep(800);
  // Long tasks in the page; frames come from a trace (a requestAnimationFrame
  // counter would itself make the page restyle every animated element each frame).
  await app.page.evaluate(() => {
    const w = window;
    w.__perf = { long: 0 };
    try {
      w.__perfObs = new PerformanceObserver((l) => l.getEntries().forEach((e) => (w.__perf.long += e.duration)));
      w.__perfObs.observe({ entryTypes: ["longtask"] });
    } catch {}
  });
  let cast;
  if (argv.includes("--frames")) cast = await screencast(app, `${games}-${s.name}`);
  const trace = path.join(dir, `${games}-${s.name}.trace.json`);
  const full = argv.includes("--trace");
  await app.browser.startTracing(app.page, {
    path: trace,
    categories: full
      ? ["devtools.timeline", "disabled-by-default-devtools.timeline", "blink", "cc", "gpu", "viz", "benchmark", "disabled-by-default-devtools.timeline.frame", "blink.animations"]
      : ["cc", "benchmark", "disabled-by-default-devtools.timeline.frame", "devtools.timeline", "toplevel"],
  });
  for (const b of s.hold) await app.pad.send(`down ${b}`);
  await sleep(seconds * 1000);
  for (const b of s.hold) await app.pad.send(`up ${b}`);
  await sleep(300);
  await cast?.stop();
  await app.browser.stopTracing();
  const p = await app.page.evaluate(() => {
    const w = window;
    w.__perfObs?.disconnect();
    return { long: w.__perf.long, dom: document.getElementsByTagName("*").length };
  });
  const cdp = await app.cdp();
  const heap = await cdp.send("Runtime.getHeapUsage");
  const mem = await app.mem();
  const r = {
    games, name: s.name, ...frames(trace), longTaskMs: p.long,
    heapMB: heap.usedSize / 2 ** 20, dom: p.dom, webviewMB: webviewPrivateMB(), coreMB: mem.privateBytes / 2 ** 20,
  };
  console.log(`${games} ${s.name}: ${r.fps.toFixed(0)} fps, ${r.dropped} dropped, ${r.holes} with holes, main thread ${r.mainMs.toFixed(0)} ms`);
  // Back to Home for the next scenario.
  for (let k = 0; k < 3; k++) await app.pad.press("east", 300);
  if (!s.desktop) await app.pad.press("lb", 300), await app.pad.press("lb", 300);
  return r;
}

// Frames from the trace: what the compositor did with each one, and
// whether it was drawn with holes (tiles not rasterized yet: flicker).
function frames(file) {
  const data = JSON.parse(fs.readFileSync(file, "utf8"));
  const ev = Array.isArray(data) ? data : data.traceEvents;
  const out = { fps: 0, presented: 0, partial: 0, dropped: 0, holes: 0, mainMs: 0 };
  const names = new Map();
  for (const e of ev) if (e.ph === "M" && e.name === "thread_name") names.set(`${e.pid}:${e.tid}`, e.args.name);
  let t0 = Infinity;
  let t1 = 0;
  for (const e of ev) {
    if (e.name === "PipelineReporter" && e.ph === "b") {
      const a = e.args?.frame_reporter ?? {};
      if (a.state === "STATE_PRESENTED_ALL") out.presented++;
      else if (a.state === "STATE_PRESENTED_PARTIAL") out.partial++;
      else if (a.state === "STATE_DROPPED") out.dropped++;
      if (a.checkerboarded_needs_raster || a.checkerboarded_needs_record) out.holes++;
      t0 = Math.min(t0, e.ts);
      t1 = Math.max(t1, e.ts);
    }
    if (e.ph === "X" && (e.name === "RunTask" || e.name === "ThreadControllerImpl::RunTask") && names.get(`${e.pid}:${e.tid}`) === "CrRendererMain") out.mainMs += e.dur / 1000;
  }
  if (t1 > t0) out.fps = ((out.presented + out.partial) / (t1 - t0)) * 1e6;
  if (!argv.includes("--trace")) fs.rmSync(file, { force: true });
  return out;
}

// WebView2's processes for Seaglass (browser, GPU, renderers): private memory.
function webviewPrivateMB() {
  const ps = `Get-CimInstance Win32_Process -Filter "Name='msedgewebview2.exe'" | Where-Object { $_.CommandLine -like '*Seaglass*' } | ForEach-Object { (Get-Process -Id $_.ProcessId -ErrorAction SilentlyContinue).PrivateMemorySize64 } | Measure-Object -Sum | ForEach-Object { $_.Sum }`;
  try {
    return Number(execFileSync("powershell", ["-NoProfile", "-Command", ps], { encoding: "utf8" }).trim()) / 2 ** 20;
  } catch {
    return 0;
  }
}

// Records the screen while a scenario runs (every frame the compositor sends).
async function screencast(app, name) {
  const cdp = await app.cdp();
  const out = path.join(dir, "frames", name);
  fs.rmSync(out, { recursive: true, force: true });
  fs.mkdirSync(out, { recursive: true });
  let n = 0;
  const onFrame = async (f) => {
    fs.writeFileSync(path.join(out, `${String(n++).padStart(4, "0")}-${Math.round(f.metadata.timestamp * 1000) % 1e7}.jpg`), Buffer.from(f.data, "base64"));
    await cdp.send("Page.screencastFrameAck", { sessionId: f.sessionId }).catch(() => {});
  };
  cdp.on("Page.screencastFrame", onFrame);
  await cdp.send("Page.startScreencast", { format: "jpeg", quality: 60, maxWidth: 960, maxHeight: 540, everyNthFrame: 1 });
  return {
    stop: async () => {
      await cdp.send("Page.stopScreencast").catch(() => {});
      cdp.off("Page.screencastFrame", onFrame);
    },
  };
}
