import { describe, expect, it } from "vitest";
import { achievementsSummary, unlockedPct, masked, rarityText, recentUnlocks, shown, sortAchievements, statusText, unlockedText } from "./achievements";
import type { Achievement, Achievements } from "./types";

const a = (id: string, p: Partial<Achievement> = {}): Achievement => ({ id, name: id, unlocked: false, ...p });
const list = (items: Achievement[], p: Partial<Achievements> = {}): Achievements => ({
  gameId: 1, source: "steam", total: items.length, unlocked: items.filter((x) => x.unlocked).length, items, updatedAt: 0, ...p,
});

describe("achievementsSummary", () => {
  it("counts and explains", () => {
    expect(achievementsSummary(null)).toBeNull();
    const s = achievementsSummary(list([a("A", { unlocked: true }), a("B"), a("C")]))!;
    expect(s.text).toBe("1 / 3 · 33%");
    expect(s.pct).toBe(33);
    expect(s.tone).toBe("muted");
    expect(achievementsSummary(list([a("A", { unlocked: true })]))?.tone).toBe("ok");
    expect(achievementsSummary(list([a("A")], { hint: "Sign in" }))?.tone).toBe("warn");
    expect(achievementsSummary(list([], { hint: "Add a key" }))).toMatchObject({ text: "No achievements yet", detail: "Add a key" });
    expect(achievementsSummary(list([]))?.text).toBe("No achievements");
    expect(achievementsSummary(list([], { source: "", hint: "Seaglass can't read the Xbox app" }))?.text).toBe("Not available");
  });
  it("only says 100% when all are unlocked", () => {
    const items = Array.from({ length: 200 }, (_, i) => a(String(i), { unlocked: i > 0 }));
    expect(achievementsSummary(list(items))?.text).toBe("199 / 200 · 99%");
  });
});

describe("sortAchievements", () => {
  it("puts newest unlocks first, then the most common locked ones", () => {
    const items = [
      a("lockedRare", { percent: 2 }),
      a("old", { unlocked: true, unlockedAt: 100 }),
      a("lockedUnknown"),
      a("undated", { unlocked: true }),
      a("new", { unlocked: true, unlockedAt: 200 }),
      a("lockedCommon", { percent: 60 }),
    ];
    expect(sortAchievements(items).map((x) => x.id)).toEqual(["new", "old", "undated", "lockedCommon", "lockedRare", "lockedUnknown"]);
    expect(recentUnlocks(items, 2).map((x) => x.id)).toEqual(["new", "old"]);
  });
});

describe("hidden achievements", () => {
  it("are masked until unlocked, unless asked", () => {
    const h = a("S", { name: "Secret ending", desc: "Spoiler", hidden: true });
    expect(masked(h, false)).toBe(true);
    expect(shown(h, false).name).toBe("Hidden achievement");
    expect(shown(h, false).desc).not.toContain("Spoiler");
    expect(shown(h, true).name).toBe("Secret ending");
    expect(shown({ ...h, unlocked: true }, false).name).toBe("Secret ending");
  });
});

describe("texts", () => {
  it("reads rarity, status and counts", () => {
    expect(rarityText(a("A"))).toBe("");
    expect(rarityText(a("A", { percent: 1.234 }))).toBe("Rare · 1.2% of players");
    expect(rarityText(a("A", { percent: 42.6 }))).toBe("43% of players");
    expect(statusText(a("A", { unlocked: true }))).toBe("Unlocked");
    expect(statusText(a("A", { unlocked: true, unlockedAt: 1_700_000_000 }))).toMatch(/^Unlocked .*2023$/); // "14 Nov 2023", "Nov 14, 2023": the date follows the locale
    expect(statusText(a("A", { progress: 3, max: 10 }))).toBe("3 / 10");
    expect(statusText(a("A"))).toBe("");
    expect(unlockedText(1)).toBe("1 achievement unlocked");
    expect(unlockedText(3)).toBe("3 achievements unlocked");
  });
});

describe("unlockedPct", () => {
  it("rounds down, so 100% means every one", () => {
    expect(unlockedPct(list([]))).toBe(0);
    expect(unlockedPct(list([a("A", { unlocked: true }), a("B"), a("C")]))).toBe(33);
    const almost = list(Array.from({ length: 200 }, (_, i) => a(`${i}`, { unlocked: i > 0 })));
    expect(unlockedPct(almost)).toBe(99);
  });
});
