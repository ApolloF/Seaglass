// Measures what Seaglass costs the PC in each state: memory (working set
// and private bytes) of the core and its WebView2 processes, CPU time,
// context switches, disk I/O, handles and threads over an idle minute,
// plus the Go heap and goroutines. Run it before and after a change, the
// same way, and compare.
//
//   node footprint.mjs [--games=2000] [--idle=60] [--only=tray,desktop,bigpicture,scan,game] [--label=before]
//
// The made-up games get generated art (in %LOCALAPPDATA%\Seaglass\art,
// named footprint-*), so the interface has real images to decode.
// Results go to %TEMP%\wl-harness\footprint-<label>.json and a table is
// printed. %APPDATA%\Seaglass is backed up and restored (the scan uses it).
import crypto from "node:crypto";
import { execFileSync, spawn } from "node:child_process";
import fs from "node:fs";
import path from "node:path";
import zlib from "node:zlib";
import { EXE, OUT, PadClient, fakeGameExe, isRunning, makeDevData, sleep, startApp } from "./lib.mjs";

const argv = process.argv.slice(2);
const opt = (k, d) => argv.find((a) => a.startsWith(`--${k}=`))?.split("=")[1] ?? d;
const GAMES = Number(opt("games", 2000));
const IDLE = Number(opt("idle", 60)) * 1000;
const SETTLE = 20000;
const ONLY = opt("only", "tray,desktop,bigpicture,scan,game").split(",");
const LABEL = opt("label", "run");
const ART_SETS = 300;
const ART_DIR = path.join(process.env.LOCALAPPDATA, "Seaglass", "art");
const dir = path.join(OUT, `footprint-${LABEL}`);
fs.mkdirSync(dir, { recursive: true });

// ---- art ----

function png(w, h, seed) {
  const row = w * 3 + 1;
  const raw = Buffer.alloc(row * h);
  for (let y = 0; y < h; y++) {
    raw[y * row] = 0;
    for (let x = 0; x < w; x++) {
      const o = y * row + 1 + x * 3;
      raw[o] = (x * 255) / w + seed * 37;
      raw[o + 1] = (y * 255) / h + seed * 91;
      raw[o + 2] = ((x >> 5) ^ (y >> 5)) & 1 ? 200 : 40 + seed;
    }
  }
  const chunk = (type, data) => {
    const len = Buffer.alloc(4);
    len.writeUInt32BE(data.length);
    const td = Buffer.concat([Buffer.from(type), data]);
    const crc = Buffer.alloc(4);
    crc.writeUInt32BE(zlib.crc32(td));
    return Buffer.concat([len, td, crc]);
  };
  const ihdr = Buffer.alloc(13);
  ihdr.writeUInt32BE(w, 0);
  ihdr.writeUInt32BE(h, 4);
  ihdr[8] = 8;
  ihdr[9] = 2;
  return Buffer.concat([Buffer.from([137, 80, 78, 71, 13, 10, 26, 10]), chunk("IHDR", ihdr), chunk("IDAT", zlib.deflateSync(raw, { level: 1 })), chunk("IEND", Buffer.alloc(0))]);
}

/** Art sets for the made-up games: distinct images, the sizes the stores give. */
function makeArt() {
  fs.mkdirSync(ART_DIR, { recursive: true });
  const kinds = { cover: [600, 900], hero: [1920, 620], backdrop: [1920, 1080], tile: [600, 600], logo: [640, 360] };
  const sets = [];
  for (let i = 0; i < ART_SETS; i++) {
    const m = {};
    for (const [k, [w, h]] of Object.entries(kinds)) {
      const name = crypto.createHash("sha1").update(`footprint-${k}-${i}`).digest("hex") + ".png";
      const file = path.join(ART_DIR, name);
      if (!fs.existsSync(file)) fs.writeFileSync(file, png(w, h, i));
      m[k] = `/art/${name}`;
    }
    sets.push(m);
  }
  return sets;
}

function devData(name, settings) {
  const data = makeDevData(name, { extra: GAMES, settings: { welcomed: true, ...settings } });
  const file = path.join(data, "library.json");
  const lib = JSON.parse(fs.readFileSync(file, "utf8"));
  let k = 0;
  for (const g of lib.games) if (g.key.startsWith("c:\\test\\made-up")) g.meta = { ...art[k++ % art.length], description: `Made-up game number ${k}.` };
  fs.writeFileSync(file, JSON.stringify(lib));
  return data;
}

// ---- measuring ----

function snapshot() {
  const ps = `$p = Get-CimInstance Win32_Process | Select-Object ProcessId,ParentProcessId,Name,WorkingSetSize,PrivatePageCount,KernelModeTime,UserModeTime,HandleCount,ThreadCount,ReadTransferCount,WriteTransferCount
$t = Get-CimInstance Win32_PerfRawData_PerfProc_Thread | Select-Object IDProcess,ContextSwitchesPersec
@{p=$p;t=$t} | ConvertTo-Json -Compress -Depth 3`;
  const j = JSON.parse(execFileSync("powershell", ["-NoProfile", "-Command", ps], { encoding: "utf8", maxBuffer: 64 << 20 }));
  const core = j.p.find((p) => p.Name === "Seaglass.exe");
  if (!core) return null;
  const tree = new Set([core.ProcessId]);
  for (let grew = true; grew; ) {
    grew = false;
    for (const p of j.p) if (!tree.has(p.ProcessId) && tree.has(p.ParentProcessId)) tree.add(p.ProcessId), (grew = true);
  }
  const cs = {};
  for (const t of j.t) if (tree.has(t.IDProcess)) cs[t.IDProcess] = (cs[t.IDProcess] ?? 0) + Number(t.ContextSwitchesPersec);
  const side = (ps) => ({
    n: ps.length,
    ws: ps.reduce((a, p) => a + Number(p.WorkingSetSize), 0),
    priv: ps.reduce((a, p) => a + Number(p.PrivatePageCount), 0),
    cpu: ps.reduce((a, p) => a + (Number(p.KernelModeTime) + Number(p.UserModeTime)) / 1e4, 0), // ms
    cs: ps.reduce((a, p) => a + (cs[p.ProcessId] ?? 0), 0),
    io: ps.reduce((a, p) => a + Number(p.ReadTransferCount) + Number(p.WriteTransferCount), 0),
    handles: ps.reduce((a, p) => a + Number(p.HandleCount), 0),
    threads: ps.reduce((a, p) => a + Number(p.ThreadCount), 0),
    by: Object.fromEntries(ps.map((p) => [p.ProcessId, { cpu: (Number(p.KernelModeTime) + Number(p.UserModeTime)) / 1e4, cs: cs[p.ProcessId] ?? 0 }])),
  });
  const web = j.p.filter((p) => tree.has(p.ProcessId) && p.ProcessId !== core.ProcessId && p.Name === "msedgewebview2.exe");
  return { at: Date.now(), core: side([core]), web: side(web) };
}

/** Idle for IDLE ms; what the core and WebView2 used in that time. */
async function idle(pad, name) {
  const a = snapshot();
  await sleep(IDLE);
  const b = snapshot();
  const secs = (b.at - a.at) / 1000;
  const mem = JSON.parse(await pad.send("mem"));
  await pad.send(`profile ${path.join(dir, name)}`);
  // Processes that came and went in between (WebView2 helpers) are left out
  // of the rates: only those alive at both ends count.
  const rate = (k, f) => Object.entries(b[k].by).reduce((t, [pid, v]) => t + (a[k].by[pid] ? v[f] - a[k].by[pid][f] : 0), 0) / secs;
  return {
    coreWS: b.core.ws, corePriv: b.core.priv, webN: b.web.n, webWS: b.web.ws, webPriv: b.web.priv,
    coreCPU: (b.core.cpu - a.core.cpu) / secs, webCPU: rate("web", "cpu"),
    coreCS: (b.core.cs - a.core.cs) / secs, webCS: rate("web", "cs"),
    coreIO: (b.core.io - a.core.io) / secs, coreHandles: b.core.handles, coreThreads: b.core.threads,
    heap: mem.heapAlloc, heapHeld: mem.heapHeld, goroutines: mem.goroutines,
  };
}

function spawnApp(args) {
  if (isRunning()) throw new Error("Seaglass is already running: close it first");
  const p = spawn(EXE, args, { detached: true, stdio: "ignore" });
  p.unref();
  return p;
}

async function stopApp(pad) {
  try {
    await pad.send("quit");
  } catch {}
  pad.close();
  for (let k = 0; k < 50 && isRunning(); k++) await sleep(200);
  if (isRunning()) execFileSync("taskkill", ["/IM", "Seaglass.exe", "/F"]);
  try {
    execFileSync("taskkill", ["/IM", "FakeGame.exe", "/F"], { stdio: "ignore" });
  } catch {}
}

// ---- scenarios ----

async function tray() {
  const data = devData("fp-tray", {});
  const t0 = Date.now();
  spawnApp(["--tray", `--dev-data=${data}`]);
  const pad = await PadClient.connect();
  const startMs = Date.now() - t0;
  await sleep(SETTLE);
  const r = await idle(pad, "tray");
  await stopApp(pad);
  return { ...r, startMs };
}

async function windowed(name, settings, browse) {
  const app = await startApp({ data: devData(`fp-${name}`, settings), pad: settings.startInBigPicture ? "ps" : null });
  await sleep(SETTLE / 2);
  await browse(app);
  // DevTools stays out of the idle minute.
  await app.browser.close().catch(() => {});
  await sleep(SETTLE / 2);
  const r = await idle(app.pad, name);
  await stopApp(app.pad);
  return { ...r, startMs: app.startMs };
}

const desktop = () =>
  windowed("desktop", {}, async (app) => {
    for (let k = 0; k < 6; k++) await app.page.mouse.wheel(0, 900), await sleep(400);
    for (let k = 0; k < 6; k++) await app.page.mouse.wheel(0, -900), await sleep(200);
  });

const bigpicture = () =>
  windowed("bigpicture", { startInBigPicture: true }, async (app) => {
    for (let k = 0; k < 20; k++) await app.pad.press("right", 150);
    await app.pad.press("down", 400);
    for (let k = 0; k < 10; k++) await app.pad.press("right", 150);
  });

async function game() {
  const data = devData("fp-game", {});
  fs.writeFileSync(path.join(OUT, "games", "Direct", "fakegame.txt"), "--run=600 --title=Fake_Direct");
  const app = await startApp({ data, pad: "ps" });
  await sleep(3000);
  await app.page.getByRole("button", { name: "Fake Direct", exact: true }).first().click();
  await sleep(400);
  await app.page.getByRole("button", { name: "Play", exact: true }).first().click();
  for (let k = 0; k < 120; k++) {
    const st = await app.state();
    if (st.session.phase === "running" && !st.mainOpen) break;
    await sleep(500);
  }
  await app.browser.close().catch(() => {});
  await sleep(SETTLE);
  const st = await app.state();
  const r = await idle(app.pad, "game");
  await stopApp(app.pad);
  return { ...r, note: `phase ${st.session.phase}, window ${st.mainOpen ? "open" : "closed"}` };
}

/** A first scan of 500 game folders, in %APPDATA%\Seaglass (not frozen). */
async function scanRun() {
  const root = path.join(OUT, "scan-games");
  const exe = fakeGameExe();
  if (!fs.existsSync(path.join(root, "Game 499"))) {
    for (let k = 0; k < 500; k++) {
      const d = path.join(root, `Game ${k}`, "Bin");
      fs.mkdirSync(d, { recursive: true });
      for (const f of ["Game.exe", "Launcher.exe"]) if (!fs.existsSync(path.join(d, f))) fs.linkSync(exe, path.join(d, f));
      for (let j = 0; j < 20; j++) fs.writeFileSync(path.join(d, `data${j}.pak`), "x");
    }
  }
  const data = path.join(process.env.APPDATA, "Seaglass");
  fs.rmSync(data, { recursive: true, force: true });
  fs.mkdirSync(data, { recursive: true });
  fs.writeFileSync(path.join(data, "settings.json"), JSON.stringify({ folders: [root], autoFolders: false, autoUpdate: false, startSyncer: false, syncProfile: false, noticeExternal: false, welcomed: true }));
  const t0 = Date.now();
  spawnApp(["--tray", "--remote-debugging=9333"]);
  const pad = await PadClient.connect();
  let peakWS = 0, peakPriv = 0, games = 0;
  const first = snapshot();
  while (Date.now() - t0 < 120000) {
    const s = snapshot();
    if (s) (peakWS = Math.max(peakWS, s.core.ws)), (peakPriv = Math.max(peakPriv, s.core.priv));
    games = JSON.parse(await pad.send("mem")).games;
    if (games >= 500) break;
  }
  const done = snapshot();
  const log = fs.readFileSync(path.join(data, "seaglass.log"), "utf8").split("\n").find((l) => l.includes(" scan: ")) ?? "";
  const mem = JSON.parse(await pad.send("mem"));
  await pad.send(`profile ${path.join(dir, "scan")}`);
  await stopApp(pad);
  return {
    coreWS: peakWS, corePriv: peakPriv, coreCPU: done.core.cpu - first.core.cpu, coreIO: done.core.io - first.core.io,
    coreHandles: done.core.handles, coreThreads: done.core.threads, heap: mem.heapAlloc, heapHeld: mem.heapHeld, goroutines: mem.goroutines,
    note: `${games} games; ${log.trim().slice(20)}`,
  };
}

// ---- run ----

const art = makeArt();
const results = {};
for (const [name, fn] of Object.entries({ tray, desktop, bigpicture, scan: scanRun, game })) {
  if (!ONLY.includes(name)) continue;
  process.stdout.write(`${name}… `);
  try {
    results[name] = await fn();
    console.log("done");
  } catch (e) {
    console.log(`failed: ${e.message}`);
    results[name] = { error: e.message };
    if (isRunning()) execFileSync("taskkill", ["/IM", "Seaglass.exe", "/F"]);
  }
}
fs.writeFileSync(path.join(OUT, `footprint-${LABEL}.json`), JSON.stringify({ exe: EXE, games: GAMES, idleSecs: IDLE / 1000, results }, null, 1));

const MB = (b) => (b === undefined ? "" : (b / 1048576).toFixed(1));
const n1 = (v) => (v === undefined ? "" : v.toFixed(1));
console.log(`\n${LABEL}: ${GAMES} games, ${IDLE / 1000} s idle\n`);
console.log("| State | Core WS / private MB | WebView2 procs, WS / private MB | CPU ms/s core / WebView2 | Ctx sw/s core / WebView2 | Core I/O KB/s | Handles / threads | Go heap MB / goroutines | Start ms |");
console.log("|---|---|---|---|---|---|---|---|---|");
for (const [k, r] of Object.entries(results)) {
  if (r.error) {
    console.log(`| ${k} | failed: ${r.error} |||||||||`);
    continue;
  }
  console.log(
    `| ${k} | ${MB(r.coreWS)} / ${MB(r.corePriv)} | ${r.webN ? `${r.webN}, ${MB(r.webWS)} / ${MB(r.webPriv)}` : "–"} | ${n1(r.coreCPU)} / ${r.webN ? n1(r.webCPU) : "–"} | ${n1(r.coreCS)} / ${r.webN ? n1(r.webCS) : "–"} | ${r.coreIO === undefined ? "" : (r.coreIO / 1024).toFixed(1)} | ${r.coreHandles} / ${r.coreThreads} | ${MB(r.heap)} / ${r.goroutines} | ${r.startMs ?? ""} |${r.note ? ` ${r.note}` : ""}`,
  );
}
