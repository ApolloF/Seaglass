import { describe, expect, it } from "vitest";
import { ago, bytes, playtime, clock, scanned, totalLine } from "./format";

describe("format", () => {
  it("playtime", () => {
    expect(playtime(0)).toBe("Not played");
    expect(playtime(600)).toBe("10 min");
    expect(playtime(3.5 * 3600)).toBe("3.5 h");
    expect(playtime(297 * 3600)).toBe("297 h");
  });
  it("bytes", () => {
    expect(bytes(83.2e9)).toBe("83.2 GB");
    expect(bytes(512)).toBe("512 B");
    expect(bytes(0)).toBe("");
  });
  it("ago", () => {
    const now = new Date(2026, 8, 25, 21, 0).getTime() / 1000;
    expect(ago(now - 60, now)).toBe("Today");
    expect(ago(now - 86400, now)).toBe("Yesterday");
    expect(ago(now - 3 * 86400, now)).toBe("3 days ago");
    expect(ago(now - 400 * 86400, now)).toBe("Last year");
    expect(ago(0, now)).toBe("Never");
  });
});

describe("clock", () => {
  it("formats a running time", () => {
    expect(clock(0)).toBe("0:00");
    expect(clock(65)).toBe("1:05");
    expect(clock(3723)).toBe("1:02:03");
  });
});

describe("totalLine", () => {
  it("says less than a minute rather than rounding up", () => {
    expect(totalLine(0)).toBe("Less than a minute in total.");
    expect(totalLine(59)).toBe("Less than a minute in total.");
    expect(totalLine(60)).toBe("1 min in total.");
    expect(totalLine(12 * 3600)).toBe("12 h in total.");
  });
});

describe("scanned", () => {
  it("says how long ago the last scan ran", () => {
    const now = 1_800_000_000;
    expect(scanned(0, now)).toBe("not scanned yet");
    expect(scanned(now - 30, now)).toBe("scanned just now");
    expect(scanned(now + 30, now)).toBe("scanned just now");
    expect(scanned(now - 5 * 60, now)).toBe("scanned 5 min ago");
    expect(scanned(now - 3 * 3600 - 59, now)).toBe("scanned 3 h ago");
  });
});
