import { describe, expect, it } from "vitest";
import { WHEEL_STEP, wheelStepper, type WheelInput } from "./wheel";

const ev = (p: Partial<WheelInput>): WheelInput => ({ deltaX: 0, deltaY: 0, deltaMode: 0, shiftKey: false, timeStamp: 0, ...p });

describe("wheelStepper", () => {
  it("turns a mouse notch into one step each way", () => {
    const step = wheelStepper();
    expect(step(ev({ deltaY: 100, timeStamp: 0 }))).toEqual(["down"]);
    expect(step(ev({ deltaY: -100, timeStamp: 50 }))).toEqual(["up"]);
  });

  it("adds small touchpad deltas up to a step", () => {
    const step = wheelStepper();
    const out = [0, 10, 20, 30].flatMap((t) => step(ev({ deltaY: WHEEL_STEP / 3, timeStamp: t })));
    expect(out).toEqual(["down"]);
  });

  it("starts afresh after a pause", () => {
    const step = wheelStepper();
    expect(step(ev({ deltaY: WHEEL_STEP / 2, timeStamp: 0 }))).toEqual([]);
    expect(step(ev({ deltaY: WHEEL_STEP / 2, timeStamp: 1000 }))).toEqual([]);
  });

  it("counts line and page deltas in pixels", () => {
    expect(wheelStepper()(ev({ deltaY: 3, deltaMode: 1 }))).toEqual(["down", "down"]);
    expect(wheelStepper()(ev({ deltaY: 1, deltaMode: 2 }))).toEqual(["down", "down", "down"]);
  });

  it("caps a flick and doesn't save up what was dropped", () => {
    const step = wheelStepper();
    expect(step(ev({ deltaY: 2000, timeStamp: 0 }))).toHaveLength(3);
    expect(step(ev({ deltaY: 1, timeStamp: 10 }))).toEqual([]);
  });

  it("steps sideways for a horizontal wheel or with Shift", () => {
    expect(wheelStepper()(ev({ deltaX: 100 }))).toEqual(["right"]);
    expect(wheelStepper()(ev({ deltaY: -100, shiftKey: true }))).toEqual(["left"]);
  });
});
