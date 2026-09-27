// The frontend's one door to the Go side. In mock mode (`npm run dev:mock`)
// the same interface is served by made-up data, so the interface can be
// built and checked in a normal browser. Vite drops the unused one.
import type { Accounts, AddonGame, AddonView, AppInfo, ArtChoice, ArtKind, Game, MetaState, PadRaw, PadState, Saves, ScanState, Session, Settings, Startup, StoreHit, SyncerStatus, UpdateState } from "./types";
import { realApi } from "./api.real";
import { mockApi } from "./api.mock";

export interface Api {
  games(): Promise<Game[]>;
  scanState(): Promise<ScanState>;
  rescan(): Promise<void>;
  setFavorite(id: number, on: boolean): Promise<Game>;
  setHidden(id: number, on: boolean): Promise<Game>;
  setPadMode(id: number, mode: string): Promise<Game>;
  rename(id: number, title: string): Promise<Game>;
  confirmMatch(id: number): Promise<Game>;
  chooseExe(id: number): Promise<Game>;
  openFolder(id: number): Promise<void>;
  /** Asks the game's store to install it. */
  install(id: number): Promise<void>;
  metaState(): Promise<MetaState>;
  refreshMetadata(id: number): Promise<void>;
  searchSteam(query: string): Promise<StoreHit[]>;
  setMatch(id: number, appId: number, name: string): Promise<Game>;
  /** Pictures the game's art of a kind could be, current first (asks the stores: seconds). */
  artChoices(id: number, kind: ArtKind): Promise<ArtChoice[]>;
  /** Makes a picture from artChoices the game's art, kept through refreshes. */
  setArt(id: number, kind: ArtKind, art: string): Promise<Game>;
  /** Puts the game in these collections (and out of the rest). */
  setCollections(id: number, names: string[]): Promise<Game>;
  /** Renames a collection in every game; "" deletes it (the games stay). */
  renameCollection(old: string, name: string): Promise<void>;

  settings(): Promise<Settings>;
  saveSettings(s: Settings): Promise<Settings>;
  addFolder(): Promise<Settings>;
  removeFolder(path: string): Promise<Settings>;
  autoFolders(): Promise<string[]>;
  info(): Promise<AppInfo>;
  openLog(): Promise<void>;
  /** Puts a diagnostics report (no keys, user folder shortened) on the clipboard. */
  copyDiagnostics(): Promise<void>;
  /** Opens a new GitHub issue. */
  reportProblem(): Promise<void>;
  /** Logs an error the interface ran into. */
  reportUIError(message: string): void;
  hasSteamGridDBKey(): Promise<boolean>;
  setSteamGridDBKey(key: string): Promise<void>;
  startWithWindows(): Promise<Startup>;
  setStartWithWindows(on: boolean): Promise<Startup>;

  onLibraryChanged(cb: () => void): () => void;
  /** Some games changed (metadata, a favorite, playtime); the rest didn't. */
  onGamesUpdated(cb: (games: Game[]) => void): () => void;
  onScanState(cb: (s: ScanState) => void): () => void;
  onMetaState(cb: (s: MetaState) => void): () => void;

  updates: {
    state(): Promise<UpdateState>;
    /** Looks for a new version now (and downloads it). */
    check(): void;
    /** Installs the downloaded update and restarts Seaglass. */
    install(): Promise<void>;
    openReleasePage(): Promise<void>;
    onState(cb: (s: UpdateState) => void): () => void;
  };

  saves: {
    /** Syncer's view of a game's saves; fresh skips the short cache. */
    get(id: number, fresh?: boolean): Promise<Saves>;
    openSyncer(): Promise<void>;
    /** Downloads Syncer's latest release and installs it (or updates it). */
    installSyncer(): Promise<void>;
    /** Opens Syncer's home page. */
    syncerProject(): Promise<void>;
    /** Syncer's state; start starts it (without its window) when it isn't running. */
    syncer(start: boolean): Promise<SyncerStatus>;
  };

  accounts: {
    get(): Promise<Accounts>;
    sync(): void;
    setSteamKey(key: string): Promise<Accounts>;
    openSteamKeyPage(): Promise<void>;
    setGOG(on: boolean): Promise<Accounts>;
    openEpicSignIn(): Promise<void>;
    epicSignIn(pasted: string): Promise<Accounts>;
    epicSignOut(): Promise<Accounts>;
    onChange(cb: (a: Accounts) => void): () => void;
  };

  addons: {
    list(): Promise<AddonView[]>;
    /** Turns an add-on on, approving the program with this SHA-256. */
    enable(id: string, sha256: string): Promise<AddonView>;
    disable(id: string): Promise<AddonView>;
    add(): Promise<AddonView | null>;
    remove(id: string): Promise<void>;
    openFolder(): Promise<void>;
    forGame(id: number): Promise<AddonGame[]>;
    runAction(id: number, addon: string, action: string): Promise<string>;
    onProgress(cb: (addon: string, text: string) => void): () => void;
  };

  launch: {
    play(id: number): Promise<void>;
    session(): Promise<Session>;
    skip(stepId: string): void;
    answer(questionId: number, option: string): void;
    cancel(): void;
    quitGame(): Promise<void>;
    setUIMode(mode: "desktop" | "bigpicture"): void;
    closeOverlay(): void;
    openMain(): void;
    onSession(cb: (s: Session) => void): () => void;
    /** Controller actions while the in-game overlay shows. */
    onOverlayAction(cb: (action: string, repeat: boolean) => void): () => void;
    /** The tray asks for a mode. */
    onUIMode(cb: (mode: "desktop" | "bigpicture") => void): () => void;
  };

  window: {
    minimise(): void;
    toggleMaximise(): void;
    close(): void;
    fullscreen(on: boolean): void;
  };

  pad: {
    state(): Promise<PadState>;
    rumble(effect: "tick" | "bump" | "confirm" | "error" | "launch"): void;
    setLight(hex: string): void;
    onAction(cb: (action: string, repeat: boolean) => void): () => void;
    onState(cb: (s: PadState) => void): () => void;
    /** Streams every button and axis (onRaw) while on, for the test screen. */
    testInput(on: boolean): void;
    onRaw(cb: (r: PadRaw) => void): () => void;
  };
}

export const api: Api = import.meta.env.VITE_MOCK === "1" ? mockApi : realApi;
