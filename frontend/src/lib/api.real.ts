import { Events, Window } from "@wailsio/runtime";
import { AccountsService, AddonsService, LaunchService, LibraryService, PadService, SavesService, SettingsService, UpdateService } from "../../bindings/github.com/ApolloF/WaterLauncher/internal/app";
import type { Api } from "./api";
import type { Accounts, AddonGame, AddonView, AppInfo, ArtChoice, Game, MetaState, PadRaw, PadState, Saves, ScanState, Session, Settings, Startup, StoreHit, SyncerStatus, UpdateState } from "./types";

// The generated bindings return the Go structs; their JSON matches ./types.
const g = (p: Promise<unknown>) => p as Promise<Game>;

export const realApi: Api = {
  games: () => LibraryService.Games() as Promise<unknown> as Promise<Game[]>,
  scanState: () => LibraryService.ScanState() as Promise<unknown> as Promise<ScanState>,
  rescan: () => LibraryService.Rescan(),
  setFavorite: (id, on) => g(LibraryService.SetFavorite(id, on)),
  setHidden: (id, on) => g(LibraryService.SetHidden(id, on)),
  setPadMode: (id, mode) => g(LibraryService.SetPadMode(id, mode)),
  rename: (id, title) => g(LibraryService.Rename(id, title)),
  confirmMatch: (id) => g(LibraryService.ConfirmMatch(id)),
  chooseExe: (id) => g(LibraryService.ChooseExe(id)),
  openFolder: (id) => LibraryService.OpenFolder(id),
  install: (id) => LibraryService.Install(id),
  metaState: () => LibraryService.MetaState() as Promise<unknown> as Promise<MetaState>,
  refreshMetadata: (id) => LibraryService.RefreshMetadata(id),
  searchSteam: (q) => LibraryService.SearchSteam(q).then((h) => (h ?? []) as unknown as StoreHit[]),
  setMatch: (id, appId, name) => g(LibraryService.SetMatch(id, appId, name)),
  artChoices: (id, kind) => LibraryService.ArtChoices(id, kind).then((c) => (c ?? []) as unknown as ArtChoice[]),
  setArt: (id, kind, art) => g(LibraryService.SetArt(id, kind, art)),
  setCollections: (id, names) => g(LibraryService.SetCollections(id, names)),
  renameCollection: (old, name) => LibraryService.RenameCollection(old, name),

  settings: () => SettingsService.Get() as Promise<unknown> as Promise<Settings>,
  saveSettings: (s) => SettingsService.Save(s as never) as Promise<unknown> as Promise<Settings>,
  addFolder: () => SettingsService.AddFolder() as Promise<unknown> as Promise<Settings>,
  removeFolder: (p) => SettingsService.RemoveFolder(p) as Promise<unknown> as Promise<Settings>,
  autoFolders: () => SettingsService.AutoFolders().then((f) => f ?? []),
  info: () => SettingsService.Info() as Promise<unknown> as Promise<AppInfo>,
  openLog: () => SettingsService.OpenLog(),
  copyDiagnostics: () => SettingsService.CopyDiagnostics(),
  reportProblem: () => SettingsService.ReportProblem(),
  reportUIError: (m) => void SettingsService.ReportUIError(m).catch(() => {}),
  hasSteamGridDBKey: () => SettingsService.HasSteamGridDBKey(),
  setSteamGridDBKey: (k) => SettingsService.SetSteamGridDBKey(k),
  startWithWindows: () => SettingsService.StartWithWindows() as Promise<unknown> as Promise<Startup>,
  setStartWithWindows: (on) => SettingsService.SetStartWithWindows(on) as Promise<unknown> as Promise<Startup>,

  onLibraryChanged: (cb) => Events.On("library:changed", () => cb()),
  onGamesUpdated: (cb) => Events.On("games:updated", (e) => cb((e.data ?? []) as unknown as Game[])),
  onScanState: (cb) => Events.On("scan:state", (e) => cb(e.data as unknown as ScanState)),
  onMetaState: (cb) => Events.On("meta:state", (e) => cb(e.data as unknown as MetaState)),

  updates: {
    state: () => UpdateService.State() as Promise<unknown> as Promise<UpdateState>,
    check: () => void UpdateService.Check(),
    install: () => UpdateService.Install(),
    openReleasePage: () => UpdateService.OpenReleasePage(),
    onState: (cb) => Events.On("update:state", (e) => cb(e.data as unknown as UpdateState)),
  },

  saves: {
    get: (id, fresh = false) => SavesService.Saves(id, fresh) as Promise<unknown> as Promise<Saves>,
    openSyncer: () => SavesService.OpenSyncer(),
    installSyncer: () => SavesService.InstallSyncer(),
    syncerProject: () => SavesService.SyncerProject(),
    syncer: (start) => SavesService.Syncer(start) as Promise<unknown> as Promise<SyncerStatus>,
  },

  accounts: {
    get: () => AccountsService.Get() as Promise<unknown> as Promise<Accounts>,
    sync: () => void AccountsService.Sync(),
    setSteamKey: (k) => AccountsService.SetSteamKey(k) as Promise<unknown> as Promise<Accounts>,
    openSteamKeyPage: () => AccountsService.OpenSteamKeyPage(),
    setGOG: (on) => AccountsService.SetGOG(on) as Promise<unknown> as Promise<Accounts>,
    openEpicSignIn: () => AccountsService.OpenEpicSignIn(),
    epicSignIn: (p) => AccountsService.EpicSignIn(p) as Promise<unknown> as Promise<Accounts>,
    epicSignOut: () => AccountsService.EpicSignOut() as Promise<unknown> as Promise<Accounts>,
    onChange: (cb) => Events.On("accounts:changed", (e) => cb(e.data as unknown as Accounts)),
  },

  addons: {
    list: () => AddonsService.List() as Promise<unknown> as Promise<AddonView[]>,
    enable: (id, sha) => AddonsService.Enable(id, sha) as Promise<unknown> as Promise<AddonView>,
    disable: (id) => AddonsService.Disable(id) as Promise<unknown> as Promise<AddonView>,
    add: () => AddonsService.Add().then((v) => ((v as unknown as AddonView)?.id ? (v as unknown as AddonView) : null)),
    remove: (id) => AddonsService.Remove(id),
    openFolder: () => AddonsService.OpenFolder(),
    forGame: (id) => AddonsService.ForGame(id).then((r) => (r ?? []) as unknown as AddonGame[]),
    runAction: (id, addon, action) => AddonsService.RunAction(id, addon, action),
    onProgress: (cb) =>
      Events.On("addon:progress", (e) => {
        const d = e.data as unknown as { addon: string; text: string };
        cb(d.addon, d.text);
      }),
  },

  launch: {
    play: (id) => LaunchService.Play(id),
    session: () => LaunchService.Session() as Promise<unknown> as Promise<Session>,
    skip: (id) => void LaunchService.Skip(id),
    answer: (q, o) => void LaunchService.Answer(q, o),
    cancel: () => void LaunchService.Cancel(),
    quitGame: () => LaunchService.QuitGame(),
    setUIMode: (m) => void LaunchService.SetUIMode(m),
    closeOverlay: () => void LaunchService.CloseOverlay(),
    openMain: () => void LaunchService.OpenMain(),
    onSession: (cb) => Events.On("launch:session", (e) => cb(e.data as unknown as Session)),
    onOverlayAction: (cb) =>
      Events.On("overlay:action", (e) => {
        const d = e.data as unknown as { action: string; repeat: boolean };
        cb(d.action, d.repeat);
      }),
    onUIMode: (cb) => Events.On("ui:mode", (e) => cb(e.data as unknown as "desktop" | "bigpicture")),
  },

  window: {
    minimise: () => void Window.Minimise(),
    toggleMaximise: () => void Window.ToggleMaximise(),
    close: () => void Window.Close(),
    fullscreen: (on) => void (on ? Window.Fullscreen() : Window.UnFullscreen()),
  },

  pad: {
    state: () => PadService.State() as Promise<unknown> as Promise<PadState>,
    rumble: (effect) => void PadService.Rumble(effect),
    setLight: (hex) => void PadService.SetLight(hex),
    onAction: (cb) =>
      Events.On("pad:action", (e) => {
        const d = e.data as unknown as { action: string; repeat: boolean };
        cb(d.action, d.repeat);
      }),
    onState: (cb) => Events.On("pad:state", (e) => cb(e.data as unknown as PadState)),
    testInput: (on) => void PadService.TestInput(on),
    onRaw: (cb) => Events.On("pad:raw", (e) => cb(e.data as unknown as PadRaw)),
  },
};
