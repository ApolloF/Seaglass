// Seaglass's Xbox controller (two drivers) in words, for Settings.
import type { ControllerStatus } from "./types";

export interface ControllerSummary {
  detail: string;
  ok: boolean;
  /** Whether the Set up button shows. */
  canSetUp: boolean;
}

export function controllerSummary(s: ControllerStatus | null): ControllerSummary {
  if (!s) return { detail: "Checking…", ok: false, canSetUp: false };
  if (s.restart) return { detail: "Restart Windows to finish setting up.", ok: false, canSetUp: false };
  if (s.viGEm)
    return {
      detail: s.canHide ? "Ready." : "Ready. Your controller can't be hidden from games yet.",
      ok: true,
      // Setting up only installs what's missing: an old or switched-off
      // HidHide is the person's to update or switch on.
      canSetUp: !s.hidHide,
    };
  return { detail: "Needs two small drivers (ViGEmBus and HidHide).", ok: false, canSetUp: true };
}

export const SET_UP_DETAIL = "Downloads the official installers. Windows asks for permission for each.";
