// Shared pieces of the harness: backing up Seaglass's data, making a
// test library, starting the real app with its dev flags, and talking to
// it over CDP (the WebView2 pages) and the dev pipe (virtual controller).
import { spawn, execFileSync } from "node:child_process";
import fs from "node:fs";
import net from "node:net";
import os from "node:os";
import path from "node:path";
import { chromium } from "playwright-core";

export const ROOT = path.resolve(import.meta.dirname, "../..");
export const EXE = process.env.WL_EXE || path.join(ROOT, "bin", "Seaglass.exe");
export const OUT = process.env.WL_OUT || path.join(os.tmpdir(), "wl-harness");
export const CDP_PORT = 9333;
const PIPE = "\\\\.\\pipe\\seaglass-dev";
const DATA = path.join(process.env.APPDATA, "Seaglass");
const BACKUP = path.join(OUT, "appdata-backup");
const MARK = path.join(OUT, "appdata-backup.json");

export const sleep = (ms) => new Promise((r) => setTimeout(r, ms));
fs.mkdirSync(OUT, { recursive: true });

// ---- %APPDATA%\Seaglass backup ----

/** Copies %APPDATA%\Seaglass aside. A backup left by a run that
 * didn't finish is restored first, never overwritten. */
export function backupAppData() {
  // Another run (or Seaglass itself) is going: its data isn't ours to touch.
  if (isRunning()) throw new Error("Seaglass is already running: close it (or the other harness run) first");
  if (fs.existsSync(MARK)) {
    console.log("an earlier run left a backup: restoring it first");
    restoreAppData({ keep: true });
  }
  fs.rmSync(BACKUP, { recursive: true, force: true });
  const existed = fs.existsSync(DATA);
  if (existed) fs.cpSync(DATA, BACKUP, { recursive: true });
  fs.writeFileSync(MARK, JSON.stringify({ from: DATA, at: new Date().toISOString(), existed, files: existed ? countFiles(BACKUP) : 0 }));
}

/** Puts %APPDATA%\Seaglass back as it was. The backup lives in Temp,
 * which Windows may have cleaned since: when it's gone or has fewer files
 * than were copied, the real data stays as it is. What a run left in its
 * place is moved aside, not deleted, until the backup is back; with keep
 * (a backup an earlier run left, perhaps long ago) it stays aside, since
 * Seaglass may have saved newer data there since. */
export function restoreAppData({ keep = false } = {}) {
  if (!fs.existsSync(MARK)) return;
  const m = JSON.parse(fs.readFileSync(MARK, "utf8"));
  if (m.existed && !(typeof m.files === "number" && fs.existsSync(BACKUP) && countFiles(BACKUP) >= m.files)) {
    fs.renameSync(MARK, `${MARK}.broken`);
    console.error(`the backup of ${DATA} in ${BACKUP} is gone or incomplete: left ${DATA} as it is`);
    return;
  }
  const aside = `${DATA}.harness-${Date.now()}`;
  if (fs.existsSync(DATA)) fs.renameSync(DATA, aside);
  try {
    if (m.existed) fs.cpSync(BACKUP, DATA, { recursive: true });
  } catch (e) {
    fs.rmSync(DATA, { recursive: true, force: true });
    if (fs.existsSync(aside)) fs.renameSync(aside, DATA);
    throw e;
  }
  if (keep && fs.existsSync(aside)) console.log(`kept what was in ${DATA} in ${aside}`);
  else fs.rmSync(aside, { recursive: true, force: true });
  fs.rmSync(MARK);
  console.log("restored", DATA);
}

function countFiles(dir) {
  return fs.readdirSync(dir, { recursive: true, withFileTypes: true }).filter((e) => e.isFile()).length;
}

/** The real library (from the backup), for its art. */
export function realLibrary() {
  for (const p of [path.join(OUT, "real-library.json"), path.join(BACKUP, "library.json"), path.join(DATA, "library.json")]) {
    try {
      return JSON.parse(fs.readFileSync(p, "utf8"));
    } catch {}
  }
  return { version: 1, nextId: 1, games: [] };
}

// ---- test data ----

/** Builds the fake game exe (tools/fakegame) once. */
export function fakeGameExe() {
  const exe = path.join(OUT, "fakegame.exe");
  execFileSync("go", ["build", "-ldflags", "-H windowsgui", "-o", exe, "./tools/fakegame"], { cwd: ROOT, stdio: "inherit" });
  return exe;
}

/** Fake games for each launch route: folder, exe and fakegame.txt flags. */
export const FAKE_GAMES = [
  { name: "Direct", title: "Fake Direct", flags: "" },
  { name: "Handover", title: "Fake Handover", flags: "--launcher" },
  { name: "Slow", title: "Fake Slow Start", flags: "--slow=8" },
  { name: "Crash", title: "Fake Crash", flags: "--crash=5" },
];

function makeFakeGames(dir) {
  const exe = fakeGameExe();
  return FAKE_GAMES.map((f) => {
    const d = path.join(dir, f.name);
    fs.rmSync(d, { recursive: true, force: true });
    fs.mkdirSync(d, { recursive: true });
    let main = path.join(d, "FakeGame.exe");
    fs.copyFileSync(exe, main);
    if (f.flags.includes("--launcher")) {
      // The launcher is the main exe; the game lives in Game\.
      fs.mkdirSync(path.join(d, "Game"));
      fs.copyFileSync(exe, path.join(d, "Game", "FakeGame.exe"));
      fs.renameSync(main, path.join(d, "Launcher.exe"));
      main = path.join(d, "Launcher.exe");
    }
    fs.writeFileSync(path.join(d, "fakegame.txt"), `${f.flags} --title=${f.title.replaceAll(" ", "_")}`);
    return { ...f, dir: d, exe: main };
  });
}

const WORDS = ["Ash", "Brine", "Cinder", "Dusk", "Echo", "Fable", "Glass", "Harbor", "Iris", "Jade", "Kiln", "Lark", "Moss", "North", "Onyx", "Pale", "Quill", "Rift", "Slate", "Thorn"];

/** Makes a data folder for --dev-data: the real games (for their art),
 * the fake games, and `extra` made-up games that borrow the real art. */
export function makeDevData(name, { extra = 0, settings = {}, fake = true } = {}) {
  const dir = path.join(OUT, "data", name);
  fs.rmSync(dir, { recursive: true, force: true });
  fs.mkdirSync(dir, { recursive: true });
  const real = realLibrary().games.filter((g) => g.installed);
  const now = Math.floor(Date.now() / 1000);
  const games = [];
  let id = 1;
  const fakes = fake ? makeFakeGames(path.join(OUT, "games")) : [];
  // The real games keep their art and details, but start the fake game:
  // a test must never start a real game, go through Steam or change
  // Steam's shortcuts (Steam Input route).
  for (const g of real) {
    const { launchUri, ...rest } = g;
    games.push({ ...rest, id: id++, padMode: "native", exe: fakes[0]?.exe ?? "", workDir: fakes[0]?.dir ?? "" });
  }
  {
    for (const f of fakes) {
      games.push({
        id: id++, key: f.dir.toLowerCase(), title: f.title, sortTitle: f.title.toLowerCase(), source: "folder",
        sourceLabel: "Folder", external: false, installed: true, dir: f.dir, exe: f.exe, workDir: f.dir,
        how: "Game folder (test)", matchHow: "Test game", confidence: 100, needsReview: false,
        addedAt: now - 5 * 86400, initial: true, seenAt: now, padMode: "native",
      });
    }
  }
  const art = real.map((g) => g.meta).filter((m) => m && m.cover);
  for (let k = 0; k < extra; k++) {
    const t = `${WORDS[k % WORDS.length]} ${WORDS[(k * 7 + 3) % WORDS.length]} ${Math.floor(k / WORDS.length) + 1}`;
    const m = art.length ? { ...art[k % art.length], description: `Made-up game number ${k + 1}.` } : undefined;
    games.push({
      id: id++, key: `c:\\test\\made-up ${k}`, title: t, sortTitle: t.toLowerCase(), source: ["steam", "epic", "gog", "folder"][k % 4],
      sourceLabel: ["Steam", "Epic", "GOG", "Folder"][k % 4], external: false, installed: true, dir: `C:\\Test\\Made-up ${k}`,
      how: "Made up (test)", matchHow: "Made up", confidence: 100, needsReview: false, addedAt: now - (k + 30) * 86400,
      initial: true, seenAt: now, playtime: (k % 5) * 3600, lastPlayed: k % 3 ? now - (k + 2) * 86400 : undefined,
      favorite: k % 17 === 0, sizeBytes: (k % 9) * 7e9, padMode: "native", meta: m,
    });
  }
  fs.writeFileSync(path.join(dir, "library.json"), JSON.stringify({ version: 1, nextId: id, games }, null, 1));
  // settings: null starts without a settings file, like a new install.
  if (settings !== null) {
    const s = { folders: [], autoFolders: false, autoUpdate: false, startSyncer: false, syncSavesBefore: false, backupSavesAfter: false, noticeExternal: false, ...settings };
    fs.writeFileSync(path.join(dir, "settings.json"), JSON.stringify(s, null, 1));
  }
  return dir;
}

// ---- the app ----

/** Starts Seaglass with its dev flags and connects to its window. */
export async function startApp({ data, pad = "ps", args = [] } = {}) {
  if (isRunning()) throw new Error("Seaglass is already running: close it first");
  const flags = [`--remote-debugging=${CDP_PORT}`, ...args];
  if (data) flags.push(`--dev-data=${data}`);
  if (pad) flags.push(`--virtual-pad=${pad}`);
  const proc = spawn(EXE, flags, { detached: true, stdio: "ignore" });
  proc.unref();
  const t0 = Date.now();
  let browser;
  for (let k = 0; k < 100 && !browser; k++) {
    await sleep(200);
    try {
      browser = await chromium.connectOverCDP(`http://127.0.0.1:${CDP_PORT}`, { timeout: 2000 });
    } catch {}
  }
  if (!browser) throw new Error("no DevTools connection");
  const app = new App(browser, proc);
  app.page = await app.waitPage((u) => !u.includes("view=overlay"));
  await app.page.waitForFunction(() => document.querySelector("#app")?.children.length, null, { timeout: 15000 });
  app.startMs = Date.now() - t0;
  app.pad = await PadClient.connect();
  return app;
}

export function isRunning() {
  try {
    return execFileSync("tasklist", ["/FI", "IMAGENAME eq Seaglass.exe", "/NH"], { encoding: "utf8" }).includes("Seaglass.exe");
  } catch {
    return false;
  }
}

export class App {
  constructor(browser, proc) {
    this.browser = browser;
    this.proc = proc;
  }

  pages() {
    return this.browser.contexts().flatMap((c) => c.pages());
  }

  /** Waits for a Seaglass page whose URL matches. */
  async waitPage(match, timeout = 30000) {
    const end = Date.now() + timeout;
    while (Date.now() < end) {
      const p = this.pages().find((p) => match(p.url()));
      if (p) return p;
      await sleep(200);
    }
    throw new Error("page not found");
  }

  /** The main window's page, after it was closed and opened again. When
   * every window closes (while a game runs), WebView2's browser process
   * ends with it, and a new one serves the window that opens next. */
  async mainPage(timeout = 30000) {
    const end = Date.now() + timeout;
    while (Date.now() < end) {
      if (!this.browser.isConnected()) {
        try {
          this.browser = await chromium.connectOverCDP(`http://127.0.0.1:${CDP_PORT}`, { timeout: 2000 });
          this.sessions = new Map();
        } catch {
          await sleep(300);
          continue;
        }
      }
      const p = this.pages().find((p) => !p.url().includes("view=overlay") && !p.isClosed());
      if (p) {
        this.page = p;
        try {
          await p.waitForFunction(() => document.querySelector("#app")?.children.length, null, { timeout: 10000 });
          return p;
        } catch {}
      }
      await sleep(300);
    }
    throw new Error("main window didn't come back");
  }

  /** Makes the page a screen of width×height pixels at a Windows display
   * scale (1.5 = 150 %): the page gets width/scale × height/scale CSS pixels. */
  async viewport(width, height, scale = 1, page = this.page) {
    const cdp = await this.cdp(page);
    await cdp.send("Emulation.setDeviceMetricsOverride", { width: Math.round(width / scale), height: Math.round(height / scale), deviceScaleFactor: scale, mobile: false });
    await sleep(250);
  }

  /** One DevTools session per page, kept open (the size emulation lives with it). */
  async cdp(page = this.page) {
    this.sessions ??= new Map();
    let s = this.sessions.get(page);
    if (!s) {
      s = await page.context().newCDPSession(page);
      this.sessions.set(page, s);
      page.once("close", () => this.sessions.delete(page));
    }
    return s;
  }

  /** A screenshot at the emulated screen's own pixels (Playwright's own
   * screenshot uses the host's display scale instead). */
  async shot(file, page = this.page) {
    fs.mkdirSync(path.dirname(file), { recursive: true });
    const cdp = await this.cdp(page);
    const { data } = await cdp.send("Page.captureScreenshot", { format: "png" });
    fs.writeFileSync(file, Buffer.from(data, "base64"));
  }

  async state() {
    return JSON.parse(await this.pad.send("state"));
  }

  async mem() {
    return JSON.parse(await this.pad.send("mem"));
  }

  /** Closes Seaglass and waits for it to go (once). */
  async quit() {
    if (this.quitting) return this.quitting;
    this.quitting = this.#quit();
    return this.quitting;
  }

  async #quit() {
    try {
      await this.pad.send("quit");
    } catch {}
    this.pad.close();
    for (let k = 0; k < 50 && isRunning(); k++) await sleep(200);
    try {
      await this.browser.close();
    } catch {}
    if (isRunning()) execFileSync("taskkill", ["/IM", "Seaglass.exe", "/F"]);
  }
}

/** The dev pipe: one command per line, one answer per line. */
export class PadClient {
  static async connect() {
    for (let k = 0; k < 50; k++) {
      try {
        const c = new PadClient();
        await c.open();
        return c;
      } catch {
        await sleep(200);
      }
    }
    throw new Error("dev pipe not available");
  }

  open() {
    return new Promise((resolve, reject) => {
      this.sock = net.connect(PIPE, () => resolve());
      this.sock.once("error", reject);
      this.buf = "";
      this.waiting = [];
      this.sock.on("data", (d) => {
        this.buf += d.toString();
        let i;
        while ((i = this.buf.indexOf("\n")) >= 0) {
          const line = this.buf.slice(0, i);
          this.buf = this.buf.slice(i + 1);
          this.waiting.shift()?.(line);
        }
      });
    });
  }

  send(cmd) {
    return new Promise((resolve, reject) => {
      if (!this.sock || this.sock.destroyed || this.sock.writableEnded) return reject(new Error("dev pipe closed"));
      this.waiting.push((line) => (line.startsWith("error:") ? reject(new Error(`${cmd}: ${line}`)) : resolve(line)));
      this.sock.write(cmd + "\n");
    });
  }

  /** Presses a button (south, east, up, …) and waits a moment for the interface. */
  async press(button, wait = 180) {
    if (button === "lt" || button === "rt") {
      // Triggers are axes.
      // A virtual trigger rests at -32768 (the gamepad reads 0 there).
      await this.send(`axis ${button} 32767`);
      await sleep(80);
      await this.send(`axis ${button} -32768`);
    } else await this.send(`press ${button}`);
    await sleep(wait);
  }

  close() {
    this.sock?.end();
  }
}
