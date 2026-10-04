import { describe, expect, it } from "vitest";
import { slowPadNotice } from "./padstate";
import type { PadState } from "./types";

const base: PadState = { connected: false, name: "", kind: "other", dualSense: false, battery: -1, wireless: false };

describe("slowPadNotice", () => {
  it("tells the person to reconnect when the controller becomes slow", () => {
    expect(slowPadNotice(base, { ...base, slow: true })).toMatch(/plug it back in/);
  });
  it("says it once, not on every later state while it stays slow", () => {
    expect(slowPadNotice({ ...base, slow: true }, { ...base, slow: true, connected: true })).toBe("");
  });
  it("says nothing for a controller that answers normally", () => {
    expect(slowPadNotice(base, { ...base, connected: true })).toBe("");
  });
});
