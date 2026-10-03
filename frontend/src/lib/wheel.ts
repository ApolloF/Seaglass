// Mouse wheel for big picture: its screens move a selection instead of
// scrolling, so the wheel turns into direction steps. A notch of a mouse
// wheel is one step; a touchpad's small, fast deltas add up to one.

export type WheelStep = "up" | "down" | "left" | "right";

/** Pixels of wheel travel per step (a mouse notch is about 100). */
export const WHEEL_STEP = 60;
/** At most this many steps from one event, so a flick doesn't fly past everything. */
const MAX_STEPS = 3;
/** A pause this long starts counting afresh. */
const IDLE_MS = 250;

const LINE_PX = 40;
const PAGE_PX = 800;

export interface WheelInput {
  deltaX: number;
  deltaY: number;
  deltaMode: number; // 0 pixels, 1 lines, 2 pages
  shiftKey: boolean;
  timeStamp: number;
}

/** Turns wheel events into steps; keep one per screen. */
export function wheelStepper() {
  let acc = 0;
  let axis: "x" | "y" = "y";
  let last = -Infinity;
  return (e: WheelInput): WheelStep[] => {
    const scale = e.deltaMode === 1 ? LINE_PX : e.deltaMode === 2 ? PAGE_PX : 1;
    let dx = e.deltaX * scale;
    let dy = e.deltaY * scale;
    // Shift turns a vertical wheel sideways, as it does in browsers.
    if (e.shiftKey && dx === 0) [dx, dy] = [dy, 0];
    const nextAxis = Math.abs(dx) > Math.abs(dy) ? "x" : "y";
    const d = nextAxis === "x" ? dx : dy;
    if (d === 0) return [];
    if (nextAxis !== axis || e.timeStamp - last > IDLE_MS || Math.sign(d) !== Math.sign(acc)) acc = 0;
    axis = nextAxis;
    last = e.timeStamp;
    acc += d;
    const whole = Math.floor(Math.abs(acc) / WHEEL_STEP);
    if (whole === 0) return [];
    // Travel beyond the cap is dropped, not saved up for later events.
    const n = Math.min(MAX_STEPS, whole);
    acc -= Math.sign(acc) * whole * WHEEL_STEP;
    const step: WheelStep = axis === "x" ? (d > 0 ? "right" : "left") : d > 0 ? "down" : "up";
    return Array(n).fill(step);
  };
}
