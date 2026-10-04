import { describe, expect, it } from "vitest";
import { accentOf, continuePlaying, metaLine, newFinds, orbitOrder, recentFirst } from "./bp";
import type { Game } from "./types";

const DAY = 86400;
const now = 1_800_000_000;
let id = 0;
const game = (name: string, daysAgo: number | null, favorite = false, installed = true) =>
  ({ id: ++id, title: name, sortTitle: name, installed, favorite, lastPlayed: daysAgo === null ? 0 : now - daysAgo * DAY }) as unknown as Game;

describe("orbitOrder", () => {
  it("puts recently played games, then favorites, in the middle", () => {
    const games = [
      game("Old", 400),
      game("Fav old", 300, true),
      game("Never", null),
      game("Yesterday", 1),
      game("Fav never", null, true),
      game("Last week", 7),
      game("Gone", 2, false, false),
    ];
    expect(orbitOrder(games, now).map((g) => g.title)).toEqual(["Yesterday", "Last week", "Fav old", "Fav never", "Old", "Never"]);
  });

  it("keeps favorites near the middle when many games were played lately", () => {
    const games = [...Array.from({ length: 30 }, (_, k) => game(`Played ${k}`, k + 1)), game("Fav", null, true)];
    const order = orbitOrder(games, now, 10);
    expect(order.findIndex((g) => g.title === "Fav")).toBe(10);
    expect(order).toHaveLength(31);
  });
});

describe("recentFirst", () => {
  it("drops games that aren't installed and breaks ties A to Z", () => {
    const games = [game("B", null), game("A", null), game("Played", 3), game("Gone", 1, false, false)];
    expect(recentFirst(games).map((g) => g.title)).toEqual(["Played", "A", "B"]);
  });
});

describe("continuePlaying", () => {
  it("lists played games, most recent first, up to n", () => {
    const games = [game("Old", 30), game("New", 1), game("Never", null), game("Mid", 5)];
    expect(continuePlaying(games, 2).map((g) => g.title)).toEqual(["New", "Mid"]);
  });

  it("falls back to the library A to Z when nothing was played", () => {
    const games = [game("C", null), game("A", null), game("B", null, false, false)];
    expect(continuePlaying(games).map((g) => g.title)).toEqual(["A", "C"]);
  });
});

describe("newFinds", () => {
  const today = Math.floor(Date.now() / 1000);
  const found = (name: string, daysAgo: number, p: Partial<Game> = {}) =>
    ({ ...game(name, null), addedAt: today - daysAgo * DAY, needsReview: false, ...p }) as Game;

  it("puts games to check first, then this week's finds, newest first", () => {
    const games = [
      found("Older find", 3),
      found("Check me", 30, { needsReview: true }),
      found("Newest find", 1),
      found("Last month", 30),
      found("First scan", 1, { initial: true }),
      found("Played", 1, { lastPlayed: today }),
      found("Not here", 1, { installed: false }),
    ];
    expect(newFinds(games).map((g) => g.title)).toEqual(["Check me", "Newest find", "Older find"]);
  });
});

describe("accentOf", () => {
  it("prefers the fetched accent, else the placeholder art's", () => {
    const g = { ...game("A", null), key: "D:\\A" } as Game;
    expect(accentOf(null)).toBeUndefined();
    expect(accentOf(g)).toMatch(/^oklch\(/);
    expect(accentOf({ ...g, meta: { accent: "#123456" } } as Game)).toBe("#123456");
  });
});

describe("metaLine", () => {
  const today = Math.floor(Date.now() / 1000);
  const g = (p: Partial<Game>) => ({ ...game("A", null), addedAt: today, ...p }) as Game;

  it("shows playtime and when it was last played", () => {
    expect(metaLine(g({ playtime: 18 * 3600, lastPlayed: today }))).toBe("18 h · Played today");
    expect(metaLine(g({ storePlaytime: 600 }))).toBe("10 min · Added today");
  });

  it("doesn't say not played when the store knows a date", () => {
    expect(metaLine(g({ storeLastPlayed: today }))).toBe("Played today");
    expect(metaLine(g({}))).toBe("Not played yet · Added today");
  });
});
