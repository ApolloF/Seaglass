// Shapes of what the Go side sends. They mirror the JSON of
// internal/library.Game, internal/settings.Settings and friends.

export interface Meta {
  description?: string;
  developers?: string[];
  publishers?: string[];
  genres?: string[];
  releaseDate?: string;
  releaseYear?: number;
  dualSense?: string; // "yes" | "no" | ""
  controller?: string;
  cover?: string;
  hero?: string;
  backdrop?: string; // 16:9, for full-screen backgrounds
  tile?: string; // square: key art with the logo, for round tiles (Orbit)
  logo?: string;
  icon?: string;
  accent?: string;
  fetchedAt?: number;
  source?: string;
  /** Art the user chose (cover, hero, backdrop, logo): kept through refreshes. */
  artOverrides?: string[];
}

export interface Game {
  id: number;
  key: string;
  title: string;
  customTitle?: string;
  sortTitle: string;
  source: string;
  sourceLabel: string;
  unofficial: boolean;
  emulator?: string;
  repacker?: string;
  drmFree?: string;
  installed: boolean;
  padHint?: string; // "libScePad" | "SDL"
  dir: string;
  exe?: string;
  args?: string;
  workDir?: string;
  launchUri?: string;
  userExe?: boolean;
  owned?: boolean; // a connected store account owns it
  installUri?: string; // asks the store to install it
  sizeBytes?: number;
  steamAppId?: number;
  metaAppId?: number;
  gogId?: string;
  epicApp?: string;
  how: string;
  matchHow: string;
  confidence: number;
  needsReview: boolean;
  confirmed?: boolean;
  addedAt: number;
  initial?: boolean;
  seenAt: number;
  lastPlayed?: number;
  playtime?: number;
  storePlaytime?: number;
  storeLastPlayed?: number;
  favorite?: boolean;
  /** The user's own groups the game is in ("Co-op", "Finished"). */
  collections?: string[];
  hidden?: boolean;
  padMode?: string; // "" | "native" | "steam"
  meta?: Meta;
}

export type Layout = "deck" | "console" | "orbit";

export interface Settings {
  folders: string[];
  autoFolders: boolean;
  detectUnofficial: boolean;
  reviewUncertain: boolean;
  showNotInstalled: boolean;
  showOwned: boolean;
  ownedGOG: boolean;
  /** Libraries whose games aren't shown (ids from SOURCE_GROUPS). */
  hiddenSources: string[];
  theme: "system" | "dark" | "light";
  bigPictureLayout: Layout;
  openBigPictureOnController: boolean;
  startInBigPicture: boolean;
  sounds: boolean;
  haptics: boolean;
  lightbar: boolean;
  psButton: boolean;
  glyphs: "auto" | "playstation" | "xbox";
  closeWhilePlaying: boolean;
  padWhilePlaying: "listen" | "off";
  noticeExternal: boolean;
  syncSavesBefore: boolean;
  backupSavesAfter: boolean;
  /** Seconds to wait for a sync before playing: 30, 60, 150 or 300. */
  syncWait: number;
  /** Start Syncer (without its window) when it isn't running. */
  startSyncer: boolean;
  autoUpdate: boolean;
  /** The first-start welcome was seen (or skipped). */
  welcomed: boolean;
}

/** One save folder Syncer looks after. */
export interface SaveFolder {
  id: string;
  label: string;
  path: string;
  sync: boolean;
  backup: boolean;
  state: string;
  needBytes: number;
  errors: number;
  conflicts: number;
  exists: boolean;
  modified: string;
  backedUp: string;
  newerOn: string;
  newerAt: string;
}

/** What Syncer knows about a game's saves. */
/** How Seaglass and Syncer get on. */
export interface SyncerStatus {
  installed: boolean;
  version?: string;
  outdated: boolean;
  connected: boolean;
  running: boolean;
  error?: string;
  syncing: boolean;
  paused: boolean;
  pausedUntil?: number;
  backingUp: boolean;
  lastBackup?: number;
  games: number;
  conflicts: number;
  checkedAt: number;
}

export interface Saves {
  installed: boolean;
  outdated?: boolean;
  available: boolean;
  error?: string;
  known: boolean;
  folders: SaveFolder[];
}

export interface ScanState {
  running: boolean;
  lastScan: number;
  tookMs: number;
  games: number;
  added: number;
  known: number;
  error?: string;
}

export interface MetaState {
  running: boolean;
  done: number;
  total: number;
}

/** A picture a game's art can be changed to (already stored). */
export interface ArtChoice {
  art: string;
  source: string;
  width: number;
  height: number;
}
export type ArtKind = "cover" | "backdrop" | "hero" | "logo";

export interface StoreHit {
  appId: number;
  name: string;
  image: string;
}

/** Every button (bit n = SDL gamepad button n) and axis of the controller in use. */
export interface PadRaw {
  buttons: number;
  axes: number[]; // left x, y, right x, y, left trigger, right trigger
}

export type PadKind = "playstation" | "xbox" | "nintendo" | "other";

export interface PadState {
  connected: boolean;
  name: string;
  kind: PadKind;
  dualSense: boolean;
  battery: number; // -1 unknown
  wireless: boolean;
  error?: string;
}

export type Phase = "preparing" | "starting" | "running" | "finishing" | "ended" | "failed" | "cancelled" | "";

export interface StepState {
  id: string;
  label: string;
  status: "pending" | "running" | "done" | "skipped" | "failed";
  detail?: string;
}

export interface Question {
  id: number;
  text: string;
  options: { id: string; label: string }[];
}

/** A game being launched or played (or the last one). */
export interface Session {
  id: number;
  gameId: number;
  title: string;
  phase: Phase;
  /** "external": started outside Seaglass and noticed. */
  route: "direct" | "store" | "steamInput" | "external" | "";
  before: StepState[];
  after: StepState[];
  question?: Question;
  startedAt?: number;
  seconds: number;
  error?: string;
  note?: string;
  /** The game's own window has come to the front (before that it's loading). */
  shown?: boolean;
  /** The exit code (0xC0000005) when the game crashed. */
  crash?: string;
}

/** Still loading: running, but its window hasn't come to the front yet
 * (some games never bring one forward themselves: after a minute it's
 * taken as running). */
export const sessionLoading = (s: Session) => s.phase === "running" && !s.shown && s.route !== "external" && s.seconds < 60;

/** What a crash says, or "" when the game quit normally. */
export const crashText = (s: Session) =>
  s.crash ? `${s.title || "The game"} closed unexpectedly (error ${s.crash}). Its playtime up to then is counted.` : "";

export const sessionActive = (s: Session | null | undefined) =>
  !!s && s.phase !== "" && s.phase !== "ended" && s.phase !== "failed" && s.phase !== "cancelled";

export interface StoreAccount {
  connected: boolean;
  available: boolean;
  name?: string;
  games: number;
  synced?: number;
  syncing: boolean;
  error?: string;
}

export interface Accounts {
  steam: StoreAccount;
  gog: StoreAccount;
  epic: StoreAccount;
}

/** Only known from a store account: never found on this PC. */
export const ownedOnly = (g: Game) => g.key.startsWith("owned:");

/** The store's name for an install button. */
export const storeName = (g: Game) => ({ steam: "Steam", gog: "GOG Galaxy", epic: "Epic" })[g.source] ?? "its store";

export interface AppInfo {
  version: string;
  dataDir: string;
  logFile: string;
  /** The previous run crashed. */
  crashedLastTime: boolean;
}

/** Mirrors internal/app.UpdateState. */
export interface UpdateState {
  current: string;
  latest?: string;
  status: "off" | "idle" | "checking" | "uptodate" | "available" | "downloading" | "ready" | "error";
  /** 0 to 1 while downloading. */
  progress: number;
  notes?: string;
  page: string;
  error?: string;
  checkedAt: number;
  /** An install of `latest` was started before and didn't take. */
  failed: boolean;
}

/** Whether Seaglass starts when you sign in to Windows. */
export interface Startup {
  on: boolean;
  /** Turned off in Task Manager's Startup apps. */
  disabledByUser: boolean;
}

export const title = (g: Game) => g.customTitle || g.title;

/** Playtime in seconds: Seaglass's own or the store's, whichever is larger. */
export const played = (g: Game) => Math.max(g.playtime ?? 0, g.storePlaytime ?? 0);

/** When the game was last played, by Seaglass or the store. */
export const lastPlayed = (g: Game) => Math.max(g.lastPlayed ?? 0, g.storeLastPlayed ?? 0);
