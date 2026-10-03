import { beforeEach, describe, expect, it, vi } from "vitest";
import type { Game, Settings } from "./types";

// The store's only import with side effects: keep the Wails runtime out.
vi.mock("./api", () => ({ api: {} }));

const { lib, errText } = await import("./store.svelte");

let id = 0;
const g = (name: string, p: Partial<Game> = {}): Game =>
  ({
    id: ++id, key: "k" + id, title: name, sortTitle: name.toLowerCase(), source: "folder", sourceLabel: "Folder",
    external: false, installed: true, needsReview: false, addedAt: 0, initial: true, ...p,
  }) as unknown as Game;

const settings = (p: Partial<Settings> = {}) => ({ showNotInstalled: false, showOwned: false, hiddenSources: [], ...p }) as unknown as Settings;
const ids = () => lib.visible.map((x) => x.title);

beforeEach(() => {
  id = 0;
  lib.filter = { kind: "all" };
  lib.query = "";
  lib.sort = "title";
  lib.settings = settings();
  lib.games = [
    g("Bravo", { collections: ["RPG"], favorite: true, lastPlayed: 100 }),
    g("Alpha", { collections: ["rpg"], lastPlayed: 200 }),
    g("Hidden", { hidden: true }),
    g("Steamy", { source: "steam", sourceLabel: "Steam" }),
    g("Uninstalled", { installed: false }),
    g("Owned", { key: "owned:gog:1", installed: false, owned: true }),
    g("Pokémon Café"),
  ];
});

describe("library views", () => {
  it("leaves hidden games, hidden libraries and uninstalled ones out", () => {
    expect(ids()).toEqual(["Alpha", "Bravo", "Pokémon Café", "Steamy"]);
    lib.settings = settings({ hiddenSources: ["steam"] });
    expect(ids()).toEqual(["Alpha", "Bravo", "Pokémon Café"]);
    expect(lib.counts.hidden).toBe(1);
    lib.filter = { kind: "hidden" };
    expect(ids()).toEqual(["Hidden"]);
  });

  it("shows owned-only games only with showOwned", () => {
    lib.settings = settings({ showNotInstalled: true });
    expect(ids()).toContain("Uninstalled");
    expect(ids()).not.toContain("Owned");
    lib.settings = settings({ showNotInstalled: true, showOwned: true });
    expect(ids()).toContain("Owned");
    expect(lib.counts.notinstalled).toBe(2);
  });

  it("treats collections differing only in case as one", () => {
    expect(lib.collections).toEqual([{ name: "RPG", count: 2 }]);
    lib.filter = { kind: "collection", name: "Rpg" };
    expect(ids()).toEqual(["Alpha", "Bravo"]);
  });

  it("matches accented titles from an unaccented query", () => {
    lib.query = "pokemon cafe";
    expect(ids()).toEqual(["Pokémon Café"]);
  });

  it("sorts the recent filter by last played, whatever the sort", () => {
    lib.filter = { kind: "recent" };
    expect(ids()).toEqual(["Alpha", "Bravo"]);
    expect(lib.counts.recent).toBe(2);
    expect(lib.counts.favorites).toBe(1);
  });
});

describe("errText", () => {
  it("unwraps the Go error", () => {
    expect(errText(new Error(JSON.stringify({ message: "not found", kind: "RuntimeError" })))).toBe("not found");
    expect(errText(new Error("plain"))).toBe("plain");
    expect(errText("just text")).toBe("just text");
  });
});

describe("saving settings", () => {
  it("keeps both of two changes made before the first is saved", async () => {
    const { api } = (await import("./api")) as unknown as { api: { saveSettings: (s: Settings) => Promise<Settings> } };
    const answers: ((s: Settings) => void)[] = [];
    api.saveSettings = (s) => new Promise((done) => answers.push(() => done(s)));
    const first = lib.saveSettings({ ...lib.settings!, showOwned: true });
    const second = lib.saveSettings({ ...lib.settings!, showNotInstalled: true });
    answers.forEach((a) => a(settings()));
    await Promise.all([first, second]);
    expect(lib.settings?.showOwned).toBe(true);
    expect(lib.settings?.showNotInstalled).toBe(true);
  });
});
