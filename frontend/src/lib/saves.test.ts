import { describe, expect, it } from "vitest";
import { savesSummary, syncerSummary } from "./saves";
import type { SaveFolder, Saves, SyncerStatus } from "./types";

const f = (p: Partial<SaveFolder>): SaveFolder => ({
  id: "g", label: "Game", path: "", sync: true, backup: true, state: "idle", needBytes: 0, errors: 0, conflicts: 0,
  exists: true, modified: "", backedUp: new Date().toISOString(), newerOn: "", newerAt: "", ...p,
});
const s = (folders: SaveFolder[], p: Partial<Saves> = {}): Saves => ({ installed: true, available: true, known: folders.length > 0, folders, ...p });

describe("savesSummary", () => {
  it("explains each state", () => {
    expect(savesSummary(null)).toBeNull();
    expect(savesSummary(s([], { installed: false }))?.action).toBe("get");
    expect(savesSummary(s([], { available: false }))?.tone).toBe("warn");
    expect(savesSummary(s([], { outdated: true, available: false }))?.action).toBe("get");
    expect(savesSummary(s([]))?.text).toBe("Not in Syncer");
    expect(savesSummary(s([f({ conflicts: 1 })]))?.text).toBe("2 versions of a save");
    expect(savesSummary(s([f({ newerOn: "TV-PC" })]))?.text).toBe("Newer save on TV-PC");
    expect(savesSummary(s([f({})]))?.text).toBe("Synced · backed up today");
    expect(savesSummary(s([f({ state: "syncing" })]))?.text).toMatch(/^Syncing…/);
    expect(savesSummary(s([f({ sync: false, state: "backup-only" })]))?.text).toBe("Backed up today · not synced");
  });
});

const st = (p: Partial<SyncerStatus> = {}): SyncerStatus => ({
  installed: true, version: "0.15.0", outdated: false, connected: true, running: true, syncing: true, paused: false,
  backingUp: false, games: 12, conflicts: 0, checkedAt: 0, ...p,
});

describe("syncerSummary", () => {
  it("gives the one thing to do about Syncer", () => {
    expect(syncerSummary(null, true)).toBeNull();
    expect(syncerSummary(st({ installed: false }), true)?.action).toBe("get");
    expect(syncerSummary(st({ outdated: true, version: undefined }), true)).toMatchObject({ action: "update", detail: "Syncer is installed; Seaglass works with 0.11 or newer." });
    expect(syncerSummary(st({ running: false }), true)).toMatchObject({ title: "Not running", tone: "muted", action: "start" });
    expect(syncerSummary(st({ running: false }), false)?.tone).toBe("warn");
    expect(syncerSummary(st({ connected: false, error: "Pipe busy" }), true)).toMatchObject({ detail: "Pipe busy", action: "open" });
    expect(syncerSummary(st({ conflicts: 1 }), true)?.title).toBe("A save has two versions");
    expect(syncerSummary(st({ conflicts: 3 }), true)?.title).toBe("3 saves have two versions");
    expect(syncerSummary(st({ paused: true }), true)?.title).toBe("Syncing paused");
    expect(syncerSummary(st({ syncing: false }), true)?.title).toBe("Connected, not syncing");
  });

  it("lists version, games and the last backup when all is well", () => {
    expect(syncerSummary(st({ games: 1 }), true)).toEqual({ title: "Connected", detail: "Syncer 0.15.0 · 1 game.", tone: "ok" });
    const today = Math.floor(Date.now() / 1000);
    expect(syncerSummary(st({ backingUp: true, lastBackup: today }), true)).toMatchObject({
      title: "Connected · backing up",
      detail: "Syncer 0.15.0 · 12 games · backed up today.",
    });
  });
});
