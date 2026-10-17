import { describe, expect, it } from "vitest";
import { needsSetup, padExplain, routeOf, translatorOf, type Translator } from "./route";
import type { Game, PadState } from "./types";

const ds: PadState = { connected: true, name: "DualSense", kind: "playstation", dualSense: true, battery: -1, wireless: false };
const xbox: PadState = { ...ds, kind: "xbox", dualSense: false };
const none: PadState = { ...ds, connected: false };
const base = { id: 1, key: "k", title: "G", sortTitle: "g", source: "folder", sourceLabel: "Folder", external: false, installed: true, dir: "D:\\G", how: "", matchHow: "", confidence: 100, needsReview: false, addedAt: 0, seenAt: 0 } as Game;

const bridge: Translator = { viGEm: true, steam: false };
const bare: Translator = { viGEm: false, steam: false };
const steam: Translator = { viGEm: false, steam: true };
const both: Translator = { viGEm: true, steam: true };
const all = [bare, bridge, steam, both];

describe("routeOf", () => {
  const xinput = { ...base, exe: "D:\\G\\g.exe", meta: { controller: "full" } };

  it("sends a store link to its store whatever the tools are", () => {
    for (const tr of all) expect(routeOf({ ...xinput, launchUri: "steam://rungameid/1" }, ds, tr)).toBe("store");
  });

  it("starts native games directly and Steam games through Steam Input", () => {
    for (const tr of all) {
      expect(routeOf({ ...xinput, padMode: "native" }, ds, tr)).toBe("direct");
      expect(routeOf({ ...base, padMode: "steam" }, xbox, tr)).toBe("steamInput");
    }
  });

  it("uses Seaglass's Xbox controller in xbox mode only when it is set up", () => {
    expect(routeOf({ ...base, padMode: "xbox" }, xbox, bridge)).toBe("bridge");
    expect(routeOf({ ...base, padMode: "xbox" }, none, both)).toBe("bridge");
    expect(routeOf({ ...base, padMode: "xbox" }, ds, bare)).toBe("direct");
    expect(routeOf({ ...base, padMode: "xbox" }, ds, steam)).toBe("direct");
  });

  it("in auto picks the chosen tool for an Xbox-only game with a PlayStation pad", () => {
    expect(routeOf(xinput, ds, bridge)).toBe("bridge");
    expect(routeOf(xinput, ds, steam)).toBe("steamInput");
    expect(routeOf(xinput, ds, both)).toBe("steamInput");
    expect(routeOf(xinput, ds, bare)).toBe("direct");
    expect(routeOf({ ...xinput, meta: { controller: "Partial" } }, ds, bridge)).toBe("bridge");
    expect(routeOf({ ...xinput, meta: { controller: "full", dualSense: "no" } }, ds, bridge)).toBe("bridge");
  });

  it("in auto starts directly when nothing needs translating", () => {
    for (const tr of all) {
      expect(routeOf(xinput, xbox, tr)).toBe("direct");
      expect(routeOf(xinput, none, tr)).toBe("direct");
      expect(routeOf({ ...xinput, meta: { controller: "full", dualSense: "yes" } }, ds, tr)).toBe("direct");
      expect(routeOf({ ...xinput, padHint: "libScePad" }, ds, tr)).toBe("direct");
      expect(routeOf({ ...xinput, meta: undefined }, ds, tr)).toBe("direct");
      expect(routeOf(base, ds, tr)).toBe("direct");
    }
  });
});

describe("needsSetup", () => {
  const xinput = { ...base, meta: { controller: "full" } };

  it("asks for setup when the game would use Seaglass's Xbox controller", () => {
    expect(needsSetup({ ...base, padMode: "xbox" }, xbox, bare)).toBe(true);
    expect(needsSetup({ ...base, padMode: "xbox" }, xbox, steam)).toBe(true);
    expect(needsSetup(xinput, ds, bare)).toBe(true);
  });

  it("does not when it is set up, Steam is chosen, or the game does not want it", () => {
    expect(needsSetup({ ...base, padMode: "xbox" }, xbox, bridge)).toBe(false);
    expect(needsSetup(xinput, ds, bridge)).toBe(false);
    expect(needsSetup(xinput, ds, steam)).toBe(false);
    expect(needsSetup(xinput, xbox, bare)).toBe(false);
    expect(needsSetup({ ...xinput, padMode: "native" }, ds, bare)).toBe(false);
    expect(needsSetup({ ...xinput, padMode: "steam" }, ds, bare)).toBe(false);
    expect(needsSetup({ ...xinput, launchUri: "steam://rungameid/1" }, ds, bare)).toBe(false);
  });
});

describe("translatorOf", () => {
  it("is empty until the status and settings are known", () => {
    expect(translatorOf(null, null)).toEqual({ viGEm: false, steam: false });
  });

  it("reads the driver and the chosen tool", () => {
    const status = { viGEm: true, hidHide: false, hidHideActive: false, canHide: false, restart: false, hidden: false };
    expect(translatorOf(status, { padTranslator: "steam" } as never)).toEqual({ viGEm: true, steam: true });
    expect(translatorOf(status, { padTranslator: "seaglass" } as never)).toEqual({ viGEm: true, steam: false });
  });
});

describe("padExplain", () => {
  const short = (g: Game, pad = ds, tr = bare) => padExplain(g, pad, tr).short;

  it("names the reason a game starts the way it does", () => {
    expect(short({ ...base, launchUri: "steam://rungameid/1" })).toBe("Steam");
    expect(short({ ...base, launchUri: "com.epicgames.launcher://apps/x?action=launch" })).toBe("Store");
    expect(short({ ...base, padMode: "native" })).toBe("Native");
    expect(short({ ...base, padMode: "steam" })).toBe("Steam Input");
    expect(short({ ...base, meta: { dualSense: "yes" } })).toBe("DualSense");
    expect(short({ ...base, padHint: "libScePad" })).toBe("DualSense");
    expect(short({ ...base, padHint: "SDL" })).toBe("Native");
    expect(short({ ...base, meta: { dualSense: "dualshock" } })).toBe("DualShock");
    expect(padExplain(base, ds, bare).long).toMatch(/no controller support known/);
  });

  it("explains the xbox mode and asks for setup when it is missing", () => {
    expect(padExplain({ ...base, padMode: "xbox" }, ds, bridge)).toEqual({
      short: "Xbox controller",
      long: "Always starts with Seaglass's Xbox controller, so your controller works as an Xbox one.",
    });
    expect(padExplain({ ...base, padMode: "xbox" }, ds, bare).long).toMatch(/Set it up in Settings first\.$/);
  });

  it("agrees with routeOf in auto", () => {
    const g = { ...base, meta: { controller: "Full" } };
    expect(routeOf(g, ds, bridge)).toBe("bridge");
    expect(padExplain(g, ds, bridge).long).toMatch(/works as an Xbox controller/);
    expect(short(g, ds, bridge)).toBe("Xbox controller");
    expect(routeOf(g, ds, steam)).toBe("steamInput");
    expect(short(g, ds, steam)).toBe("Steam Input");
    expect(short(g, ds, both)).toBe("Steam Input");
  });

  it("mentions the chosen tool when auto has nothing to translate yet", () => {
    const g = { ...base, meta: { controller: "partial" } };
    expect(padExplain(g, xbox, bare)).toMatchObject({ short: "Auto", long: expect.stringMatching(/Seaglass's Xbox controller/) });
    expect(padExplain(g, xbox, steam).long).toMatch(/Steam Input/);
    expect(padExplain(g, ds, bare).long).toMatch(/set up in Settings/);
  });
});
