import type { PadState } from "./types";

/**
 * The note to show when the controller has just become slow to answer
 * Windows (every question to it waits for a timeout, so it shows up late
 * or not at all until it's reconnected), or "" when there's nothing new.
 */
export function slowPadNotice(was: PadState, now: PadState): string {
  if (!now.slow || was.slow) return "";
  return "Windows is slow to answer your controller, so Seaglass may not see it. Unplug it and plug it back in, or turn it off and on.";
}
