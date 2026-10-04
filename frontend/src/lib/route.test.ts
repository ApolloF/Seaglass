import { describe, expect, it } from "vitest";
import { padExplain, routeOf } from "./route";
import type { Game, PadState } from "./types";

const ds: PadState = { connected: true, name: "DualSense", kind: "playstation", dualSense: true, battery: -1, wireless: false };
const xbox: PadState = { ...ds, kind: "xbox", dualSense: false };
const base = { id: 1, key: "k", title: "G", sortTitle: "g", source: "folder", sourceLabel: "Folder", external: false, installed: true, dir: "D:\\G", how: "", matchHow: "", confidence: 100, needsReview: false, addedAt: 0, seenAt: 0 } as Game;

describe("routeOf", () => {
  it("matches the Go side", () => {
    const xinput = { ...base, exe: "D:\\G\\g.exe", meta: { controller: "full" } };
    expect(routeOf({ ...xinput, launchUri: "steam://rungameid/1" }, ds)).toBe("store");
    expect(routeOf(xinput, ds)).toBe("steamInput");
    expect(routeOf(xinput, xbox)).toBe("direct");
    expect(routeOf({ ...xinput, meta: { controller: "full", dualSense: "yes" } }, ds)).toBe("direct");
    expect(routeOf({ ...xinput, padHint: "libScePad" }, ds)).toBe("direct");
    expect(routeOf({ ...xinput, padMode: "native" }, ds)).toBe("direct");
    expect(routeOf({ ...base, padMode: "steam" }, xbox)).toBe("steamInput");
    expect(routeOf(base, ds)).toBe("direct");
  });
});

describe("padExplain", () => {
  it("names the reason a game starts the way it does", () => {
    const short = (g: Game, pad = ds) => padExplain(g, pad).short;
    expect(short({ ...base, launchUri: "steam://rungameid/1" })).toBe("Steam");
    expect(short({ ...base, launchUri: "com.epicgames.launcher://apps/x?action=launch" })).toBe("Store");
    expect(short({ ...base, padMode: "native" })).toBe("Native");
    expect(short({ ...base, padMode: "steam" })).toBe("Steam Input");
    expect(short({ ...base, meta: { dualSense: "yes" } })).toBe("DualSense");
    expect(short({ ...base, padHint: "libScePad" })).toBe("DualSense");
    expect(short({ ...base, padHint: "SDL" })).toBe("Native");
    expect(short({ ...base, meta: { dualSense: "dualshock" } })).toBe("DualShock");
    expect(short({ ...base, meta: { controller: "full" } })).toBe("Steam Input");
    expect(padExplain({ ...base, meta: { controller: "partial" } }, xbox)).toMatchObject({ short: "Auto", long: expect.stringMatching(/Xbox controllers/) });
    expect(padExplain(base, ds).long).toMatch(/no controller support known/);
  });

  it("agrees with routeOf on Steam Input", () => {
    const g = { ...base, meta: { controller: "Full" } };
    expect(routeOf(g, ds)).toBe("steamInput");
    expect(padExplain(g, ds).short).toBe("Steam Input");
  });
});
