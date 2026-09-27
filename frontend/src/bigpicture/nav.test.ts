import { describe, expect, it } from "vitest";
import { clamp, gridStep, rowOffset, step } from "./nav";

describe("gridStep", () => {
  // 7 items in rows of 3:  0 1 2 / 3 4 5 / 6
  it("moves within the grid", () => {
    expect(gridStep(0, 7, 3, "right")).toBe(1);
    expect(gridStep(1, 7, 3, "down")).toBe(4);
    expect(gridStep(4, 7, 3, "up")).toBe(1);
    expect(gridStep(4, 7, 3, "left")).toBe(3);
  });
  it("stops at the edges", () => {
    expect(gridStep(0, 7, 3, "left")).toBeNull();
    expect(gridStep(2, 7, 3, "right")).toBeNull();
    expect(gridStep(1, 7, 3, "up")).toBeNull();
    expect(gridStep(6, 7, 3, "down")).toBeNull();
    expect(gridStep(6, 7, 3, "right")).toBeNull();
  });
  it("lands on the last item when the row below is short", () => {
    expect(gridStep(5, 7, 3, "down")).toBe(6);
  });
});

describe("helpers", () => {
  it("steps and clamps", () => {
    expect(step(0, 3, -1)).toBeNull();
    expect(step(1, 3, 1)).toBe(2);
    expect(clamp(9, 0, 5)).toBe(5);
  });
  it("keeps a leading item visible", () => {
    expect(rowOffset(0, 100, 20)).toBe(0);
    expect(rowOffset(1, 100, 20)).toBe(0);
    expect(rowOffset(3, 100, 20)).toBe(-240);
  });

  it("shows more items before the selection with a longer lead", () => {
    // The deck's library row keeps four covers to the left.
    expect(rowOffset(4, 200, 20, 4)).toBe(0);
    expect(rowOffset(6, 200, 20, 4)).toBe(-440);
  });
});
