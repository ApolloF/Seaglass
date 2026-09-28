import { describe, expect, it } from "vitest";
import { routeOf } from "./route";
import type { Game, PadState } from "./types";

const ds: PadState = { connected: true, name: "DualSense", kind: "playstation", dualSense: true, battery: -1, wireless: false };
const xbox: PadState = { ...ds, kind: "xbox", dualSense: false };
const base = { id: 1, key: "k", title: "G", sortTitle: "g", source: "folder", sourceLabel: "Folder", external: false, installed: true, dir: "D:\G", how: "", matchHow: "", confidence: 100, needsReview: false, addedAt: 0, seenAt: 0 } as Game;

describe("routeOf", () => {
  it("matches the Go side", () => {
    const xinput = { ...base, exe: "D:\G\g.exe", meta: { controller: "full" } };
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
