import { describe, expect, it } from "vitest";
import { orbitOrder } from "./bp";
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
