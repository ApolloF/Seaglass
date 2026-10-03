import { describe, expect, it } from "vitest";
import { artFor, hash } from "./art";

describe("hash", () => {
  it("is FNV-1a, unsigned 32-bit", () => {
    expect(hash("")).toBe(2166136261);
    expect(hash("a")).toBe(0xe40c292c);
    expect(hash("foobar")).toBe(0xbf9cf968);
  });
});

describe("artFor", () => {
  it("draws the same art for the same key", () => {
    const a = artFor("D:\Games\Same");
    expect(artFor("D:\Games\Same")).toBe(a);
    expect(structuredClone(a)).toEqual(a);
  });

  it("tells apart keys that differ only at the end", () => {
    const accents = new Set(Array.from({ length: 20 }, (_, i) => artFor(`D:\Games\Game ${i}`).accent));
    expect(accents.size).toBeGreaterThan(15);
  });

  it("fills every field with something CSS can use", () => {
    for (let i = 0; i < 50; i++) {
      const a = artFor(`key ${i}`);
      for (const [k, v] of Object.entries(a)) {
        if (typeof v === "number") expect(v, k).toBeGreaterThanOrEqual(0);
        else expect(v, k).not.toMatch(/NaN|undefined/);
      }
      expect(a.accent).toMatch(/^oklch\(0\.8 0\.13 \d+\)$/);
      expect(a.sunL).toMatch(/^\d+(\.\d)?%$/);
    }
  });
});
