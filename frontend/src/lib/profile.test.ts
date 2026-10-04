import { describe, expect, it } from "vitest";
import { accountColor, hasAccounts, initial, playing, profileSummary } from "./profile";
import type { Profile, SyncerAccount } from "./types";

const acc = (id: string, p: Partial<SyncerAccount> = {}): SyncerAccount => ({ id, name: id, active: false, ...p });
const profile = (p: Partial<Profile> = {}): Profile => ({
  enabled: true, accounts: [acc("ann"), acc("bo")], owner: "ann", installed: true, reachable: true,
  supported: true, synced: false, backup: false, dismissed: false, dir: "", ...p,
});

describe("accountColor", () => {
  it("passes only a plain six-digit hex colour to CSS", () => {
    expect(accountColor(acc("a", { color: "#1a2B3c" }))).toBe("#1a2B3c");
    expect(accountColor(acc("a", { color: "#abc" }))).toBeUndefined();
    expect(accountColor(acc("a", { color: "red;background:url(x)" }))).toBeUndefined();
    expect(accountColor(acc("a"))).toBeUndefined();
    expect(accountColor(undefined)).toBeUndefined();
  });
});

describe("initial", () => {
  it("takes the first letter, whole characters included", () => {
    expect(initial("  ann")).toBe("A");
    expect(initial("élise")).toBe("É");
    expect(initial("😀 Fun")).toBe("😀");
    expect(initial("   ")).toBe("?");
  });
});

describe("hasAccounts", () => {
  it("needs Syncer's accounts on and at least one account", () => {
    expect(hasAccounts(profile())).toBe(true);
    expect(hasAccounts(profile({ enabled: false }))).toBe(false);
    expect(hasAccounts(profile({ accounts: [] }))).toBe(false);
    expect(hasAccounts(null)).toBe(false);
  });
});

describe("playing", () => {
  it("is the active account, else the PC's owner", () => {
    expect(playing(profile({ active: "bo" }))?.id).toBe("bo");
    expect(playing(profile())?.id).toBe("ann");
    expect(playing(profile({ owner: "shared" }))).toBeUndefined();
    expect(playing(null)).toBeUndefined();
  });
});

describe("profileSummary", () => {
  it("says where the profile goes, most pressing first", () => {
    expect(profileSummary(profile(), false)).toEqual({ text: "Kept on this PC only.", tone: "muted" });
    expect(profileSummary(null, true).text).toMatch(/^Syncer isn't installed/);
    expect(profileSummary(profile({ installed: false }), true).tone).toBe("muted");
    expect(profileSummary(profile({ dataError: "Disk full", synced: true }), true)).toEqual({ text: "Disk full", tone: "warn" });
    expect(profileSummary(profile({ dismissed: true, synced: true }), true).tone).toBe("warn");
    expect(profileSummary(profile({ synced: true }), true)).toEqual({ text: "Synced with your other PCs by Syncer.", tone: "ok" });
    expect(profileSummary(profile({ synced: true, backup: true }), true).text).toBe("Synced with your other PCs and backed up by Syncer.");
    expect(profileSummary(profile({ reachable: false }), true).text).toMatch(/^Syncer isn't running/);
    expect(profileSummary(profile(), true).text).toBe("Waiting for Syncer…");
  });
});
