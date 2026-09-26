// Starts and closes the fake game on every route, from big picture with
// the controller: a game that starts directly, a launcher that hands over
// to its game, a slow start, and a crash. Records what the session and
// the windows do over time, with screenshots along the way.
//
//   node launches.mjs [--only=Direct,Handover] [--from=bigpicture|desktop]
import fs from "node:fs";
import path from "node:path";
import { FAKE_GAMES, OUT, backupAppData, makeDevData, restoreAppData, sleep, startApp } from "./lib.mjs";

const argv = process.argv.slice(2);
const opt = (k, d) => argv.find((a) => a.startsWith(`--${k}=`))?.split("=")[1] ?? d;
const only = opt("only", FAKE_GAMES.map((f) => f.name).join(",")).split(",");
const from = opt("from", "bigpicture");
// How long each fake game runs before it closes itself.
const RUN = { Direct: "--run=10", Handover: "--run=10", Slow: "--run=8", Crash: "" };

const dir = path.join(OUT, `launches-${from}`);
fs.rmSync(dir, { recursive: true, force: true });
fs.mkdirSync(dir, { recursive: true });
backupAppData();
const results = [];
let current = [];
let app;
try {
  const data = makeDevData("launches", { settings: { startInBigPicture: from === "bigpicture" } });
  // Each game closes by itself, as if the player quit.
  for (const f of FAKE_GAMES) {
    const txt = path.join(OUT, "games", f.name, "fakegame.txt");
    fs.writeFileSync(txt, `${f.flags} ${RUN[f.name]} --title=${f.title.replaceAll(" ", "_")}`);
    fs.rmSync(path.join(OUT, "games", f.name, "fakegame.log"), { force: true });
    fs.rmSync(path.join(OUT, "games", f.name, "Game", "fakegame.log"), { force: true });
  }
  app = await startApp({ data });
  await app.viewport(1920, 1080);
  await sleep(1500);
  for (const f of FAKE_GAMES.filter((f) => only.includes(f.name))) {
    try {
      results.push(await launch(f));
    } catch (e) {
      results.push({ name: f.name, verdict: `harness error: ${e.stack}`, timeline: current });
      await app.mainPage().catch(() => {});
    }
  }
} finally {
  await app?.quit();
  restoreAppData();
}
fs.writeFileSync(path.join(dir, "results.json"), JSON.stringify(results, null, 1));
for (const r of results) {
  console.log(`\n${r.name}: ${r.verdict}`);
  for (const e of r.timeline) console.log(`  ${(e.t / 1000).toFixed(1).padStart(5)} s  ${e.what}`);
}

async function launch(f) {
  const t0 = Date.now();
  const timeline = (current = []);
  const note = (what) => timeline.push({ t: Date.now() - t0, what });
  const shots = [];
  const shot = async (name) => {
    try {
      const file = path.join(dir, `${f.name}-${name}.png`);
      await app.shot(file);
      shots.push(file);
    } catch {}
  };
  // Pick the game with the controller and press ✕.
  if (from === "bigpicture") {
    await app.pad.press("rb", 700); // Library, A–Z
    for (let k = 0; k < 80; k++) {
      const focused = await app.page.evaluate(() => document.querySelector(".tile.on, button.on[aria-label]")?.getAttribute("aria-label") ?? "");
      if (focused === f.title) break;
      await app.pad.press(k < 40 ? "right" : "down", 120);
    }
  } else {
    await app.page.getByRole("button", { name: f.title, exact: true }).first().click();
    await sleep(400);
  }
  note(`picked ${f.title}`);
  if (from === "bigpicture") await app.pad.press("south", 100);
  else await app.page.getByRole("button", { name: "Play", exact: true }).first().click();
  note("pressed Play");

  // Follow the session and the windows until it's over and the interface is back.
  let last = "";
  let sawRunning = false;
  let ended = false;
  for (let k = 0; k < 150 && !ended; k++) {
    let st;
    try {
      st = await app.state();
    } catch (e) {
      note(`state: ${e.message}`);
      break;
    }
    const s = st.session;
    const line = `session ${s.phase || "-"}${s.note ? ` (${s.note})` : ""}${s.error ? ` error: ${s.error}` : ""} · window ${st.mainOpen ? "open" : "closed"}${st.gameMode ? " (game mode)" : ""} · controller ${["active", "passive", "off"][st.padMode] ?? st.padMode}`;
    if (line !== last) {
      note(line);
      last = line;
      if (st.mainOpen) {
        try {
          await app.mainPage(3000);
          await shot(`${timeline.length}-${s.phase || "idle"}`);
        } catch {}
      }
    }
    if (s.phase === "running") sawRunning = true;
    if ((s.phase === "ended" || s.phase === "failed") && st.mainOpen) {
      ended = true;
      await sleep(1500);
      await app.mainPage();
      await app.viewport(1920, 1080);
      await sleep(500);
      await shot("after");
      const summary = await app.page.evaluate(() => document.querySelector(".launch, .launching, [class*=launch]")?.textContent?.replace(/\s+/g, " ").trim().slice(0, 200) ?? "");
      note(`summary on screen: ${summary || "(none)"}`);
      // Put the summary away.
      await app.pad.press("east", 600);
    }
    await sleep(400);
  }
  const logs = [path.join(OUT, "games", f.name, "fakegame.log"), path.join(OUT, "games", f.name, "Game", "fakegame.log")]
    .filter((p) => fs.existsSync(p))
    .map((p) => fs.readFileSync(p, "utf8").trim());
  const st = await app.state();
  const verdict = !ended ? "DID NOT FINISH" : !sawRunning ? "never ran" : `ok, played ${st.session.seconds} s`;
  return { name: f.name, verdict, timeline, logs, shots };
}
