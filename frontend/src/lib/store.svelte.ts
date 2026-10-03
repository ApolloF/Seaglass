import { api } from "./api";
import type { Achievement, AppInfo, Game, MetaState, Profile, ScanState, Session, SessionAchievements, Settings, UpdateState } from "./types";
import { unlockedText } from "./achievements";
import { lastPlayed, ownedOnly, played, title } from "./types";

export type FilterKind = "all" | "installed" | "notinstalled" | "favorites" | "recent" | "found" | "hidden";
export type Filter = { kind: FilterKind } | { kind: "source"; source: string } | { kind: "collection"; name: string };

/** A collection's key: names differing only in case are one collection. */
export const collectionKey = (name: string) => name.trim().toLowerCase();
export const inCollection = (g: Game, name: string) => !!g.collections?.some((c) => collectionKey(c) === collectionKey(name));
export type Sort = "title" | "recent" | "added" | "playtime";

export interface Toast {
  id: number;
  text: string;
  tone: "info" | "error";
  /** Achievement icons to show with it. */
  icons?: Achievement[];
}

// Source groups shown in the sidebar, in this order.
export const SOURCE_GROUPS: { id: string; label: string; match: (g: Game) => boolean }[] = [
  { id: "steam", label: "Steam", match: (g) => g.source === "steam" },
  { id: "epic", label: "Epic", match: (g) => g.source === "epic" },
  { id: "gog", label: "GOG", match: (g) => g.source === "gog" },
  { id: "ea", label: "EA", match: (g) => g.source === "ea" },
  { id: "ubisoft", label: "Ubisoft", match: (g) => g.source === "ubisoft" },
  { id: "battlenet", label: "Battle.net", match: (g) => g.source === "battlenet" },
  { id: "xbox", label: "Xbox", match: (g) => g.source === "xbox" },
  { id: "external", label: "External", match: (g) => g.external },
  { id: "standalone", label: "Standalone", match: (g) => !g.external && (g.source === "installer" || g.source === "shortcut") },
  { id: "folder", label: "Folders", match: (g) => !g.external && g.source === "folder" },
];

const WEEK = 7 * 86400;

/** A game found in the last week and not played yet. */
export const isFresh = (g: Game) => !g.initial && !lastPlayed(g) && !played(g) && Date.now() / 1000 - g.addedAt < WEEK;

const norm = (s: string) => s.toLowerCase().normalize("NFKD").replace(/[^\p{L}\p{N}]+/gu, "");
// One collator for every sort: localeCompare builds one per call.
const collator = new Intl.Collator();
// What the search box matches, worked out once per game object (an
// updated game is a new object, so it's worked out again).
const searchTexts = new WeakMap<Game, string>();
function searchText(g: Game): string {
  let t = searchTexts.get(g);
  if (t === undefined) {
    t = norm(title(g)) + "|" + norm(g.sourceLabel);
    searchTexts.set(g, t);
  }
  return t;
}

class LibraryStore {
  games = $state<Game[]>([]);
  settings = $state<Settings | null>(null);
  info = $state<AppInfo | null>(null);
  scan = $state<ScanState>({ running: true, lastScan: 0, tookMs: 0, games: 0, added: 0, known: 0 });
  meta = $state<MetaState>({ running: false, done: 0, total: 0 });
  loaded = $state(false);
  /** The game being launched or played (or the last one). */
  session = $state<Session | null>(null);
  update = $state<UpdateState | null>(null);
  /** The achievements the last play session unlocked (views read theirs again). */
  achSession = $state<SessionAchievements | null>(null);
  /** Who's playing on this PC (Syncer's accounts). */
  profile = $state<Profile | null>(null);

  filter = $state<Filter>({ kind: "all" });
  sort = $state<Sort>("title");
  query = $state("");
  selectedId = $state<number | null>(null);
  toasts = $state<Toast[]>([]);

  /** Games the library views show at all: installed ones (or every one when
   * the setting says so), without hidden ones or ones from hidden libraries. */
  base = $derived.by(() => {
    const showAll = this.settings?.showNotInstalled ?? false;
    const showOwned = this.settings?.showOwned ?? false;
    const hiddenLibs = SOURCE_GROUPS.filter((s) => this.settings?.hiddenSources?.includes(s.id));
    return this.games.filter(
      (g) => !g.hidden && (g.installed || (showAll && !ownedOnly(g)) || (showOwned && !!g.owned)) && !hiddenLibs.some((s) => s.match(g)),
    );
  });

  /** The filtered, sorted list, apart from the search text: typing in
   * the search box only filters it, it doesn't sort everything again. */
  private sorted = $derived.by(() => {
    const f = this.filter;
    let list: Game[];
    if (f.kind === "hidden") list = this.games.filter((g) => g.hidden);
    else if (f.kind === "collection") list = this.base.filter((g) => inCollection(g, f.name));
    else if (f.kind === "source") {
      const group = SOURCE_GROUPS.find((s) => s.id === f.source);
      list = this.base.filter((g) => (group ? group.match(g) : true));
    } else {
      list = this.base.filter((g) => {
        switch (f.kind) {
          case "installed":
            return g.installed;
          case "notinstalled":
            return !g.installed;
          case "favorites":
            return !!g.favorite;
          case "recent":
            return lastPlayed(g) > 0;
          case "found":
            return g.needsReview || isFresh(g);
          default:
            return true;
        }
      });
    }
    const sort = f.kind === "recent" ? "recent" : this.sort;
    const byTitle = (a: Game, b: Game) => collator.compare(a.sortTitle, b.sortTitle);
    return [...list].sort((a, b) => {
      switch (sort) {
        case "recent":
          return lastPlayed(b) - lastPlayed(a) || byTitle(a, b);
        case "added":
          return b.addedAt - a.addedAt || byTitle(a, b);
        case "playtime":
          return played(b) - played(a) || byTitle(a, b);
        default:
          return byTitle(a, b);
      }
    });
  });

  visible = $derived.by(() => {
    const q = norm(this.query);
    return q ? this.sorted.filter((g) => searchText(g).includes(q)) : this.sorted;
  });

  selected = $derived.by(() => {
    const id = this.selectedId;
    return this.visible.find((g) => g.id === id) ?? this.visible[0] ?? null;
  });

  counts = $derived.by(() => {
    const b = this.base;
    return {
      all: b.length,
      installed: b.filter((g) => g.installed).length,
      notinstalled: b.filter((g) => !g.installed).length,
      favorites: b.filter((g) => g.favorite).length,
      recent: b.filter((g) => lastPlayed(g) > 0).length,
      found: b.filter((g) => g.needsReview || isFresh(g)).length,
      hidden: this.games.filter((g) => g.hidden).length,
    };
  });

  /** The user's collections, A–Z, with how many shown games each has. */
  collections = $derived.by(() => {
    const m = new Map<string, { name: string; count: number }>();
    for (const g of this.base)
      for (const c of g.collections ?? []) {
        const e = m.get(collectionKey(c)) ?? { name: c, count: 0 };
        e.count++;
        m.set(collectionKey(c), e);
      }
    return [...m.values()].sort((a, b) => a.name.localeCompare(b.name));
  });

  sources = $derived.by(() =>
    SOURCE_GROUPS.map((s) => ({ ...s, count: this.base.filter(s.match).length })).filter((s) => s.count > 0),
  );

  /** Every library with games on this PC, shown or not, for Settings. */
  libraries = $derived.by(() =>
    SOURCE_GROUPS.map((s) => ({
      ...s,
      count: this.games.filter((g) => !g.hidden && g.installed && s.match(g)).length,
      hidden: !!this.settings?.hiddenSources?.includes(s.id),
    })).filter((s) => s.count > 0 || s.hidden),
  );

  /** Shows or hides a library's games. */
  toggleLibrary(id: string) {
    const s = this.settings;
    if (!s) return;
    const hidden = s.hiddenSources.includes(id) ? s.hiddenSources.filter((h) => h !== id) : [...s.hiddenSources, id];
    if (this.filter.kind === "source" && this.filter.source === id) this.filter = { kind: "all" };
    void this.saveSettings({ ...s, hiddenSources: hidden });
  }

  async init() {
    // An event that arrives while the first snapshot is still loading is
    // newer than it: the snapshot mustn't put the old state back. (The
    // window reopens as a game closes, and its session ends a few seconds
    // later, often while the library is still loading; a stale session
    // left the game looking like it still ran.) Games changed meanwhile are
    // read again once the snapshot is in.
    const fresh = new Set<string>();
    api.onLibraryChanged(() => (fresh.add("games"), void this.refresh()));
    api.onGamesUpdated((gs) => (fresh.add("games"), gs.forEach((g) => this.replace(g, true))));
    api.onScanState((s) => ((this.scan = s), fresh.add("scan")));
    api.onMetaState((s) => ((this.meta = s), fresh.add("meta")));
    api.launch.onSession((s) => ((this.session = s), fresh.add("session")));
    api.profile.onChange((p) => {
      const was = this.profile?.owner;
      this.profile = p;
      fresh.add("profile");
      if (was && was !== p.owner && p.ownerName) this.toast(`${p.ownerName} is playing on this PC`);
    });
    api.onSettingsChanged((s) => {
      this.settings = s;
      fresh.add("settings");
    });
    api.updates.onState((s) => ((this.update = s), fresh.add("update")));
    api.achievements.onSession((s) => {
      this.achSession = s;
      if (s.unlocked.length) this.toast(`${s.title}: ${unlockedText(s.unlocked.length)}`, "info", s.unlocked.slice(0, 5));
    });
    const [games, settings, scan, info, meta, session, update, profile] = await Promise.all([
      api.games(),
      api.settings(),
      api.scanState(),
      api.info(),
      api.metaState(),
      api.launch.session(),
      api.updates.state(),
      api.profile.get(),
    ]);
    if (!fresh.has("profile")) this.profile = profile;
    if (!fresh.has("update")) this.update = update;
    this.announceVersion(info.version);
    if (info.crashedLastTime) this.toast("Seaglass closed unexpectedly last time. Settings → About → Copy diagnostics helps with a bug report.", "error");
    if (!fresh.has("meta")) this.meta = meta;
    if (!fresh.has("session")) this.session = session;
    this.games = games;
    if (fresh.has("games")) void this.refresh();
    if (!fresh.has("settings")) this.settings = settings;
    if (!fresh.has("scan")) this.scan = scan;
    this.info = info;
    this.loaded = true;
  }

  /** Says so once after Seaglass was updated. */
  private announceVersion(version: string) {
    try {
      const seen = localStorage.getItem("wl.version");
      localStorage.setItem("wl.version", version);
      if (seen && seen !== version && version !== "dev") this.toast(`Updated to Seaglass ${version}`);
    } catch {
      /* storage unavailable: skip the note */
    }
  }

  /** Installs the downloaded update; Seaglass restarts. */
  installUpdate() {
    return this.run(() => api.updates.install());
  }

  private refreshSeq = 0;
  async refresh() {
    // Two refreshes close together can resolve out of order: only the
    // newest one's list is put in place.
    const n = ++this.refreshSeq;
    try {
      const gs = await api.games();
      if (n === this.refreshSeq) this.games = gs;
    } catch (e) {
      this.toast(errText(e), "error");
    }
  }

  /** Starts a game; the session events tell how it goes. */
  play(g: Game) {
    return this.run(() => api.launch.play(g.id));
  }

  /** Puts an updated game in place without waiting for a full refresh;
   * with add, a game the list doesn't have yet joins it. */
  replace(g: Game, add = false) {
    const i = this.games.findIndex((x) => x.id === g.id);
    if (i >= 0) this.games[i] = g;
    else if (add) this.games.push(g);
  }

  /** Makes another of Syncer's accounts the one playing on this PC. */
  async switchAccount(id: string) {
    const p = await this.run(() => api.profile.switch(id));
    if (p) this.profile = p;
    return !!p;
  }

  private settingsSeq = 0;
  /** Saves the settings. They change here at once, so a second change
   * made before the first is saved builds on it instead of undoing it;
   * only the newest save's answer is kept. */
  async saveSettings(next: Settings) {
    const seq = ++this.settingsSeq;
    const before = this.settings;
    this.settings = next;
    try {
      const saved = await api.saveSettings($state.snapshot(next) as Settings);
      if (seq === this.settingsSeq) this.settings = saved;
    } catch (e) {
      if (seq === this.settingsSeq) this.settings = before;
      this.toast(errText(e), "error");
    }
  }

  /** Runs an action, showing its error as a toast. */
  async run<T>(fn: () => Promise<T>): Promise<T | undefined> {
    try {
      const r = await fn();
      if (r && typeof r === "object" && "id" in r && "key" in r) this.replace(r as unknown as Game);
      return r;
    } catch (e) {
      this.toast(errText(e), "error");
      return undefined;
    }
  }

  private nextToast = 1;
  toast(text: string, tone: Toast["tone"] = "info", icons?: Achievement[]) {
    const t = { id: this.nextToast++, text, tone, icons };
    this.toasts = [...this.toasts, t];
    setTimeout(() => (this.toasts = this.toasts.filter((x) => x.id !== t.id)), tone === "error" || icons?.length ? 7000 : 3500);
  }
}

/** Error text without the Go/wails wrapping. */
export function errText(e: unknown): string {
  const s = e instanceof Error ? e.message : typeof e === "string" ? e : JSON.stringify(e);
  try {
    const j = JSON.parse(s);
    if (j && typeof j.message === "string") return j.message;
  } catch {
    /* not JSON */
  }
  return s;
}

export const lib = new LibraryStore();
