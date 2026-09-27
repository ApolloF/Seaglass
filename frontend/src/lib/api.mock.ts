// Made-up library for `npm run dev:mock`: the games from the design canvas,
// covering every way a game can be found.
import type { Api } from "./api";
import type { Accounts, AppInfo, Game, MetaState, Saves, ScanState, Session, Settings, Startup, UpdateState } from "./types";
import { sessionActive } from "./types";

const now = Math.floor(Date.now() / 1000);
const day = 86400;

let nextId = 1;
function game(p: Partial<Game> & { title: string }): Game {
  const id = nextId++;
  return {
    id,
    key: `mock:${id}`,
    sortTitle: p.title.toLowerCase().replace(/^the /, ""),
    source: "steam",
    sourceLabel: "Steam",
    unofficial: false,
    installed: true,
    dir: `D:\\Games\\${p.title}`,
    how: "Steam library",
    matchHow: "Steam library",
    confidence: 100,
    needsReview: false,
    addedAt: now - 90 * day,
    seenAt: now,
    ...p,
  };
}

let games: Game[] = [
  game({ meta: { description: "A fallen knight climbs a burning mountain to take back a crown that was never theirs. Brutal, fair combat and a world that remembers every choice.", developers: ["Ashgrove"], publishers: ["Ashgrove"], genres: ["Action", "RPG"], releaseYear: 2026, dualSense: "yes", accent: "#e8894a" }, title: "Ember Crown", source: "installer", sourceLabel: "Unofficial · Goldberg", unofficial: true, emulator: "Goldberg", steamAppId: 1245620, how: "Game folder in D:\\Games (Steam emulator)", matchHow: "Steam AppID read from steam_settings", confidence: 95, playtime: 18 * 3600, lastPlayed: now - 3600, favorite: true, exe: "D:\\Games\\Ember Crown\\EmberCrown.exe", sizeBytes: 54e9 }),
  game({ title: "Hollow Tide", steamAppId: 413150, launchUri: "steam://rungameid/413150", playtime: 42 * 3600, lastPlayed: now - day, favorite: true, dir: "C:\\Program Files (x86)\\Steam\\steamapps\\common\\Hollow Tide", sizeBytes: 64e9 }),
  game({ title: "Neon Meridian", source: "installer", sourceLabel: "Repack · DODI", unofficial: true, repacker: "DODI", how: "Installed by a DODI repack", matchHow: "Matched by title", confidence: 85, playtime: 7 * 3600, lastPlayed: now - 3 * day, padMode: "steam", sizeBytes: 21e9 }),
  game({ title: "Starfall Protocol", source: "epic", sourceLabel: "Epic", how: "Epic Games library", matchHow: "Epic Games library", playtime: 64 * 3600, lastPlayed: now - 8 * day, sizeBytes: 38e9 }),
  game({ title: "Grimwald", source: "installer", sourceLabel: "Unofficial · EMPRESS", unofficial: true, emulator: "EMPRESS", steamAppId: 1149460, how: "Installer entry in Windows", matchHow: "Steam AppID read from steam_emu.ini", confidence: 95, playtime: 88 * 3600, lastPlayed: now - 15 * day, favorite: true, sizeBytes: 47e9 }),
  game({ title: "Quiet Harbor", source: "gog", sourceLabel: "GOG", gogId: "1207658924", how: "GOG Galaxy library", matchHow: "GOG Galaxy library", playtime: 12 * 3600, lastPlayed: now - 34 * day, sizeBytes: 9e9 }),
  game({ title: "Frostline", source: "xbox", sourceLabel: "Xbox", how: "Xbox app library", matchHow: "Xbox app library", playtime: 9 * 3600, lastPlayed: now - 40 * day, sizeBytes: 88e9 }),
  game({ title: "Iron Veil", source: "installer", sourceLabel: "Unofficial · RUNE", unofficial: true, emulator: "RUNE", repacker: "FitGirl", steamAppId: 1086940, how: "Installed by a FitGirl repack", matchHow: "Steam AppID read from steam_emu.ini", confidence: 95, addedAt: now - 3600, sizeBytes: 71e9 }),
  game({ title: "Sable Run", source: "folder", sourceLabel: "Folder", how: "Game folder in D:\\Games (Unity)", matchHow: "Not matched to a known game", confidence: 40, needsReview: true, addedAt: now - 2 * 3600, padMode: "steam", sizeBytes: 3e9 }),
  game({ title: "Lumen Drift", steamAppId: 620, launchUri: "steam://rungameid/620", addedAt: now - day, sizeBytes: 6e9 }),
  game({ title: "Kestrel", source: "steam", installed: false, playtime: 6 * 3600, lastPlayed: now - 400 * day }),
  game({ title: "Copper Fields", source: "gog", sourceLabel: "GOG", installed: false, playtime: 21 * 3600, lastPlayed: now - 700 * day }),
];

// ?games=N in the mock URL adds made-up games, to try the layouts with a
// bigger library; ?layout=console|orbit|deck picks the big picture layout.
const mockParams = new URLSearchParams(typeof location !== "undefined" ? location.search : "");
{
  const words = ["Ash", "Brine", "Cinder", "Dusk", "Echo", "Fable", "Glass", "Harbor", "Iris", "Jade", "Kiln", "Lark", "Moss", "North", "Onyx", "Pale"];
  const more = Math.min(2000, Number(mockParams.get("games")) || 0);
  for (let k = 0; k < more; k++) {
    const t = `${words[k % words.length]} ${words[(k * 7 + 3) % words.length]} ${Math.floor(k / words.length) + 1}`;
    games.push(game({ title: t, playtime: (k % 5) * 3600, lastPlayed: k % 3 ? now - (k + 2) * day : undefined, addedAt: now - (k + 30) * day, sizeBytes: (k % 9) * 7e9 }));
  }
}

// ?art=steam gives the made-up games real art from Steam's CDN (straight
// from the browser), to judge the layouts with real pictures.
if (mockParams.get("art") === "steam") {
  const ids = [1245620, 413150, 1086940, 620, 1145360, 504230, 1091500, 2358720, 1623730, 292030, 1174180, 271590, 374320, 814380, 105600, 367520, 400, 220, 2050650, 883710, 782330, 379720, 1817070, 1593500, 2215430, 1151640, 990080, 252490, 1966720, 534380, 1716740, 1551360, 2379780, 1794680, 646570, 250900, 1057090, 976730, 1196590, 239140];
  const cdn = "https://shared.akamai.steamstatic.com/store_item_assets/steam/apps/";
  games.forEach((g, k) => {
    const id = ids[k % ids.length];
    g.meta = { ...g.meta, cover: `${cdn}${id}/library_600x900_2x.jpg`, hero: `${cdn}${id}/library_hero.jpg`, backdrop: `${cdn}${id}/library_hero_2x.jpg`, logo: `${cdn}${id}/logo.png` };
  });
}

let settings: Settings = {
  folders: ["D:\\Games"],
  autoFolders: true,
  detectUnofficial: true,
  reviewUncertain: true,
  showNotInstalled: false,
  theme: "system",
  bigPictureLayout: (["console", "orbit"] as const).find((l) => l === mockParams.get("layout")) ?? "deck",
  openBigPictureOnController: true,
  startInBigPicture: false,
  sounds: false,
  haptics: true,
  lightbar: true,
  psButton: true,
  glyphs: "auto",
  closeWhilePlaying: true,
  padWhilePlaying: "listen",
  noticeExternal: true,
  showOwned: false,
  ownedGOG: false,
  hiddenSources: [],
  syncSavesBefore: true,
  backupSavesAfter: true,
  syncWait: 60,
  startSyncer: true,
  autoUpdate: true,
  // ?welcome=1 shows the first-start welcome.
  welcomed: mockParams.get("welcome") !== "1",
};

let startup: Startup = { on: false, disabledByUser: false };
let updateState: UpdateState = { current: "v1.0.0", status: "idle", progress: 0, page: "https://github.com/ApolloF/Seaglass/releases/latest", checkedAt: 0, failed: false };
const updateListeners = new Set<(s: UpdateState) => void>();
function setUpdate(p: Partial<UpdateState>) {
  updateState = { ...updateState, ...p };
  updateListeners.forEach((cb) => cb(clone(updateState)));
}
// ?update=ready in the mock URL shows a downloaded update.
if (typeof location !== "undefined" && new URLSearchParams(location.search).get("update") === "ready") {
  updateState = { ...updateState, latest: "v1.0.1", status: "ready", progress: 1, notes: "Fixes and polish.", checkedAt: Math.floor(Date.now() / 1000) };
}

const hour = 3600;
function mockSaves(g: Game | undefined): Saves {
  const base = { installed: true, available: true, known: false, folders: [] as Saves["folders"] };
  if (!g) return base;
  const iso = (s: number) => new Date(s * 1000).toISOString();
  const folder = (p: Partial<Saves["folders"][number]>) => ({
    id: g.key, label: g.title, path: "C:\\Users\\you\\AppData\\Roaming\\" + g.title, sync: true, backup: true, state: "idle",
    needBytes: 0, errors: 0, conflicts: 0, exists: true, modified: iso(now - 5 * hour), backedUp: iso(now - 2 * hour), newerOn: "", newerAt: "", ...p,
  });
  switch (g.title) {
    case "Ember Crown":
      return { ...base, known: true, folders: [folder({})] };
    case "Hollow Tide":
      return { ...base, known: true, folders: [folder({ conflicts: 2 })] };
    case "Grimwald":
      return { ...base, known: true, folders: [folder({ newerOn: "DESKTOP-TV", newerAt: iso(now - hour) })] };
    case "Quiet Harbor":
      return { ...base, known: true, folders: [folder({ sync: false, state: "backup-only" })] };
    case "Frostline":
      return { installed: false, available: false, known: false, folders: [] };
  }
  return base;
}

let sgdb = false;
let state: ScanState = { running: false, lastScan: now - 120, tookMs: 940, games: games.length, added: 0, known: 52107 };
const libListeners = new Set<() => void>();
const scanListeners = new Set<(s: ScanState) => void>();

const clone = <T>(v: T): T => JSON.parse(JSON.stringify(v));
const wait = (ms = 60) => new Promise((r) => setTimeout(r, ms));

function update(id: number, fn: (g: Game) => void): Promise<Game> {
  const g = games.find((x) => x.id === id);
  if (!g) return Promise.reject(new Error("game not found"));
  fn(g);
  libListeners.forEach((cb) => cb());
  return Promise.resolve(clone(g));
}

// ---- pretend store accounts ----

let accounts: Accounts = {
  steam: { connected: false, available: true, games: 0, syncing: false },
  gog: { connected: false, available: true, games: 0, syncing: false },
  epic: { connected: false, available: true, games: 0, syncing: false },
};

function addOwned() {
  if (games.some((g) => g.key.startsWith("owned:"))) return;
  const list: [string, number][] = [["Portal 2", 620], ["Hades", 1145360], ["Celeste", 504230]];
  for (const [title, appId] of list) {
    games.push(game({ title, key: "owned:steam:" + appId, installed: false, owned: true, steamAppId: appId, installUri: "steam://install/" + appId, how: "Owned on Steam", matchHow: "Owned on Steam", initial: true, storePlaytime: appId === 620 ? 20 * 3600 : 0 }));
  }
  libListeners.forEach((cb) => cb());
}

const padListeners = new Set<(a: string, repeat: boolean) => void>();

// ---- a pretend game session ----

let session: Session = { id: 0, gameId: 0, title: "", phase: "", route: "", before: [], after: [], seconds: 0 };
const sessionListeners = new Set<(s: Session) => void>();
let skipStep = "";
let answerWith: ((o: string) => void) | null = null;

function setSession(p: Partial<Session>) {
  session = { ...session, ...p };
  sessionListeners.forEach((cb) => cb(clone(session)));
}

async function runMockSession(g: Game) {
  const steam = g.padMode === "steam" && !g.launchUri;
  setSession({
    id: session.id + 1, gameId: g.id, title: g.customTitle || g.title, phase: "preparing",
    route: "", before: steam ? [{ id: "steamInput", label: "Steam Input", status: "running" }] : [],
    after: [], seconds: 0, startedAt: 0, error: "", note: "", question: undefined,
  });
  const id = session.id;
  if (steam) {
    const answer = await new Promise<string>((resolve) => {
      answerWith = resolve;
      setSession({ question: { id: 1, text: `Steam needs to restart once to add ${session.title} for Steam Input.`, options: [
        { id: "restart", label: "Restart Steam" }, { id: "direct", label: "Start without Steam Input" }, { id: "cancel", label: "Cancel" },
      ] } });
    });
    answerWith = null;
    setSession({ question: undefined });
    if (answer === "cancel") return setSession({ phase: "cancelled", before: [{ id: "steamInput", label: "Steam Input", status: "skipped" }] });
    setSession({ before: [{ id: "steamInput", label: "Steam Input", status: "running", detail: answer === "restart" ? "Closing Steam…" : "" }] });
    await wait(skipStep ? 0 : 1400);
    setSession({ before: [{ id: "steamInput", label: "Steam Input", status: "done", detail: answer === "restart" ? "Added to Steam" : "Starting without Steam Input" }] });
  }
  const cur = () => session; // read fresh after each await
  if (cur().id !== id || cur().phase !== "preparing") return;
  setSession({ phase: "starting", route: steam ? "steamInput" : g.launchUri ? "store" : "direct" });
  await update(g.id, (x) => (x.lastPlayed = Math.floor(Date.now() / 1000)));
  await wait(1800);
  if (cur().id !== id || cur().phase !== "starting") return;
  setSession({ phase: "running", startedAt: Math.floor(Date.now() / 1000) });
  const t = setInterval(() => {
    if (session.id !== id || session.phase !== "running") return clearInterval(t);
    setSession({ seconds: session.seconds + 1 });
  }, 1000);
}

export const mockApi: Api = {
  async games() {
    await wait();
    return clone(games);
  },
  async scanState() {
    return clone(state);
  },
  async rescan() {
    state = { ...state, running: true };
    scanListeners.forEach((cb) => cb(clone(state)));
    await wait(1200);
    state = { ...state, running: false, lastScan: Math.floor(Date.now() / 1000) };
    scanListeners.forEach((cb) => cb(clone(state)));
    libListeners.forEach((cb) => cb());
  },
  setFavorite: (id, on) => update(id, (g) => (g.favorite = on)),
  setHidden: (id, on) => update(id, (g) => (g.hidden = on)),
  setPadMode: (id, mode) => update(id, (g) => (g.padMode = mode === "auto" ? "" : mode)),
  rename: (id, t) => update(id, (g) => (g.customTitle = t.trim())),
  confirmMatch: (id) => update(id, (g) => ((g.confirmed = true), (g.needsReview = false))),
  chooseExe: (id) => update(id, (g) => ((g.exe = g.dir + "\\Game.exe"), (g.userExe = true))),
  async openFolder() {},
  async install() {},
  async metaState(): Promise<MetaState> {
    return { running: false, done: 0, total: 0 };
  },
  async refreshMetadata() {},
  async searchSteam(q) {
    await wait(300);
    return [
      { appId: 757310, name: "Sable", image: "" },
      { appId: 717850, name: `${q} Deluxe Edition`, image: "" },
      { appId: 941900, name: `${q} Soundtrack`, image: "" },
    ];
  },
  async artChoices(id, kind) {
    await wait(700);
    const key = { cover: "cover", backdrop: "backdrop", hero: "hero", logo: "logo" } as const;
    const seen = new Set<string>();
    const out = [];
    const cur = games.find((x) => x.id === id)?.meta?.[key[kind]];
    if (cur) (seen.add(cur), out.push({ art: cur, source: "Current", width: 0, height: 0 }));
    for (const x of games) {
      const a = x.meta?.[key[kind]];
      if (a && !seen.has(a) && out.length < 9) (seen.add(a), out.push({ art: a, source: x.title, width: 0, height: 0 }));
    }
    return out;
  },
  setArt: (id, kind, art) => update(id, (g) => (g.meta = { ...g.meta, [kind]: art, artOverrides: [...(g.meta?.artOverrides ?? []), kind] })),
  setCollections: (id, names) => update(id, (g) => (g.collections = [...new Set(names.map((n) => n.trim()).filter(Boolean))])),
  async renameCollection(old, name) {
    for (const g of games)
      if (g.collections?.some((c) => c.toLowerCase() === old.toLowerCase())) {
        g.collections = g.collections.map((c) => (c.toLowerCase() === old.toLowerCase() ? name : c)).filter(Boolean);
      }
    libListeners.forEach((cb) => cb());
  },
  setMatch: (id, appId, name) => update(id, (g) => ((g.steamAppId = appId), (g.title = name), (g.confirmed = true), (g.needsReview = false), (g.matchHow = "Chosen by you"), (g.confidence = 100))),

  async settings() {
    return clone(settings);
  },
  async saveSettings(s) {
    settings = clone(s);
    return clone(settings);
  },
  async addFolder() {
    settings = { ...settings, folders: [...settings.folders, "E:\\More Games"] };
    return clone(settings);
  },
  async removeFolder(p) {
    settings = { ...settings, folders: settings.folders.filter((f) => f !== p) };
    return clone(settings);
  },
  async autoFolders() {
    return ["C:\\Program Files (x86)\\DODI-Repacks", "D:\\Games"];
  },
  async info(): Promise<AppInfo> {
    return { version: "mock", dataDir: "C:\\Users\\you\\AppData\\Roaming\\Seaglass", logFile: "seaglass.log", crashedLastTime: false };
  },
  async openLog() {},
  async copyDiagnostics() {
    await navigator.clipboard?.writeText("Seaglass diagnostics (mock)").catch(() => {});
  },
  async reportProblem() {},
  reportUIError(m) {
    console.warn("interface error:", m);
  },
  async hasSteamGridDBKey() {
    return sgdb;
  },
  async setSteamGridDBKey(k) {
    sgdb = !!k;
  },
  async startWithWindows() {
    return clone(startup);
  },
  async setStartWithWindows(on) {
    startup = { ...startup, on };
    return clone(startup);
  },
  updates: {
    async state() {
      return clone(updateState);
    },
    check() {
      if (updateState.status === "downloading" || updateState.status === "checking") return;
      setUpdate({ status: "checking", error: undefined });
      setTimeout(() => {
        setUpdate({ status: "downloading", latest: "v1.0.1", notes: "Fixes and polish.", progress: 0, checkedAt: Math.floor(Date.now() / 1000) });
        let p = 0;
        const t = setInterval(() => {
          p = Math.min(1, p + 0.2);
          setUpdate({ progress: p });
          if (p >= 1) {
            clearInterval(t);
            setUpdate({ status: "ready" });
          }
        }, 300);
      }, 700);
    },
    async install() {
      await wait(400);
      location.reload();
    },
    async openReleasePage() {},
    onState(cb) {
      updateListeners.add(cb);
      return () => updateListeners.delete(cb);
    },
  },

  onLibraryChanged(cb) {
    libListeners.add(cb);
    return () => libListeners.delete(cb);
  },
  onGamesUpdated() {
    return () => {};
  },
  onScanState(cb) {
    scanListeners.add(cb);
    return () => scanListeners.delete(cb);
  },
  onMetaState() {
    return () => {};
  },
  saves: {
    async get(id) {
      await wait(250);
      return mockSaves(games.find((g) => g.id === id));
    },
    async openSyncer() {},
    async installSyncer() {
      await new Promise((r) => setTimeout(r, 1500));
    },
    async syncerProject() {},
    async syncer(start) {
      await wait(start ? 900 : 200);
      // ?syncer=missing|old|off shows the other states.
      const mode = mockParams.get("syncer");
      const at = Math.floor(Date.now() / 1000);
      if (mode === "missing") return { installed: false, outdated: false, connected: false, running: false, syncing: false, paused: false, backingUp: false, games: 0, conflicts: 0, checkedAt: at };
      if (mode === "old") return { installed: true, version: "0.9.2", outdated: true, connected: false, running: false, error: "Syncer needs an update (version 0.11.0 or newer)", syncing: false, paused: false, backingUp: false, games: 0, conflicts: 0, checkedAt: at };
      if (mode === "off" && !start) return { installed: true, version: "0.12.0", outdated: false, connected: false, running: false, syncing: false, paused: false, backingUp: false, games: 0, conflicts: 0, checkedAt: at };
      return { installed: true, version: "0.12.0", outdated: false, connected: true, running: true, syncing: true, paused: false, backingUp: false, lastBackup: at - 2 * hour, games: 42, conflicts: 1, checkedAt: at };
    },
  },
  accounts: {
    async get() {
      return clone(accounts);
    },
    sync() {},
    async setSteamKey(k) {
      await wait(600);
      if (k && !/^[0-9A-Fa-f]{32}$/.test(k)) throw new Error("that doesn't look like a Steam Web API key (32 letters and digits)");
      accounts = { ...accounts, steam: { ...accounts.steam, connected: !!k, games: k ? 214 : 0, synced: k ? Math.floor(Date.now() / 1000) : 0 } };
      if (k) addOwned();
      return clone(accounts);
    },
    async openSteamKeyPage() {},
    async setGOG(on) {
      accounts = { ...accounts, gog: { ...accounts.gog, connected: on, games: on ? 31 : 0 } };
      return clone(accounts);
    },
    async openEpicSignIn() {},
    async epicSignIn(p) {
      await wait(600);
      if (!/[0-9a-f]{32}/.test(p)) throw new Error("paste the authorizationCode the Epic page showed after signing in");
      accounts = { ...accounts, epic: { ...accounts.epic, connected: true, name: "Player One", games: 87 } };
      return clone(accounts);
    },
    async epicSignOut() {
      accounts = { ...accounts, epic: { connected: false, available: true, games: 0, syncing: false } };
      return clone(accounts);
    },
    onChange() {
      return () => {};
    },
  },
  launch: {
    async play(id) {
      const g = games.find((x) => x.id === id);
      if (!g) throw new Error("game not found");
      if (sessionActive(session)) throw new Error(`${session.title} is still running`);
      runMockSession(g);
    },
    async session() {
      return clone(session);
    },
    skip(id) {
      skipStep = id;
    },
    answer(_q, option) {
      answerWith?.(option);
    },
    cancel() {
      if (session.phase === "preparing" || session.phase === "starting") setSession({ phase: "cancelled", question: undefined });
    },
    async quitGame() {
      if (session.phase !== "running") throw new Error("no game is running");
      setSession({ phase: "finishing", after: [{ id: "savesAfter", label: "Back up saves", status: "running", detail: "Backing up…" }] });
      await wait(1500);
      setSession({ phase: "ended", after: [{ id: "savesAfter", label: "Back up saves", status: "done", detail: "Backed up" }] });
    },
    setUIMode() {},
    closeOverlay() {},
    openMain() {},
    onSession(cb) {
      sessionListeners.add(cb);
      return () => sessionListeners.delete(cb);
    },
    onOverlayAction() {
      return () => {};
    },
    onUIMode() {
      return () => {};
    },
  },
  window: { minimise() {}, toggleMaximise() {}, close() {}, fullscreen() {} },
  pad: {
    async state() {
      return { connected: true, name: "DualSense Wireless Controller", kind: "playstation", dualSense: true, battery: 82, wireless: true };
    },
    rumble() {},
    setLight() {},
    testInput() {},
    onRaw(cb) {
      // window.mockRaw(buttons, axes) shows a controller state on the test screen.
      (window as unknown as { mockRaw: (b: number, a?: number[]) => void }).mockRaw = (b, a = [0, 0, 0, 0, 0, 0]) => cb({ buttons: b, axes: a });
      return () => {};
    },
    onAction(cb) {
      // window.mockPad("down") presses a controller button, for trying things out.
      padListeners.add(cb);
      (window as unknown as { mockPad: (a: string, repeat?: boolean) => void }).mockPad = (a, repeat = false) => padListeners.forEach((f) => f(a, repeat));
      return () => padListeners.delete(cb);
    },
    onState() {
      return () => {};
    },
  },
};
