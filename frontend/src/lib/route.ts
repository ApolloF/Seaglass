// How a game will start, mirroring route() in internal/app/launch.go, so
// the interface can explain it before the game is played.
import type { ControllerStatus, Game, PadState, Settings } from "./types";

export type Route = "direct" | "store" | "steamInput" | "bridge";

/** What can turn a PlayStation controller into an Xbox one: Seaglass's own
 * controller (viGEm: its driver is there) and whether Steam is the chosen tool. */
export type Translator = { viGEm: boolean; steam: boolean };

export function translatorOf(status: ControllerStatus | null, settings: Settings | null): Translator {
  return { viGEm: status?.viGEm ?? false, steam: settings?.padTranslator === "steam" };
}

/** Auto mode would hand the game an Xbox controller: a PlayStation controller
 * is in use and the game supports controllers but not PlayStation ones. */
function wantsXbox(g: Game, pad: PadState): boolean {
  if (g.console) return false; // the emulator reads the controller itself
  if (!pad.connected || pad.kind !== "playstation" || g.padHint || !g.meta) return false;
  const ds = g.meta.dualSense ?? "";
  if (ds && ds !== "no") return false;
  const c = (g.meta.controller ?? "").toLowerCase();
  return c === "full" || c === "partial";
}

export function routeOf(g: Game, pad: PadState, tr: Translator): Route {
  if (g.launchUri) return "store";
  if (g.padMode === "native") return "direct";
  if (g.padMode === "steam") return "steamInput";
  if (g.padMode === "xbox") return tr.viGEm ? "bridge" : "direct";
  if (!wantsXbox(g, pad)) return "direct";
  if (tr.steam) return "steamInput";
  return tr.viGEm ? "bridge" : "direct";
}

/** The game would get Seaglass's Xbox controller if it were set up. */
export function needsSetup(g: Game, pad: PadState, tr: Translator): boolean {
  if (g.launchUri || tr.viGEm) return false;
  return g.padMode === "xbox" || (!g.padMode && !tr.steam && wantsXbox(g, pad));
}

/** The controller mode in words: a short label and a sentence. */
export function padExplain(g: Game, pad: PadState, tr: Translator): { short: string; long: string } {
  const mode = g.padMode || "auto";
  if (g.launchUri) {
    return g.launchUri.startsWith("steam:")
      ? { short: "Steam", long: "Starts through Steam, which applies its own Steam Input settings." }
      : { short: "Store", long: "Starts through its store; the game handles the controller." };
  }
  if (mode === "native") return { short: "Native", long: "Always starts directly. The game handles the controller itself." };
  if (mode === "steam") return { short: "Steam Input", long: "Always starts through Steam Input, so a DualSense acts as an Xbox controller." };
  if (mode === "xbox")
    return {
      short: "Xbox controller",
      long: "Always starts with Seaglass's Xbox controller, so your controller works as an Xbox one." + (tr.viGEm ? "" : " Set it up in Settings first."),
    };
  if (g.console) return { short: "Emulator", long: `Auto: ${g.console.emulator} reads the controller itself, so the game starts directly.` };
  const ds = g.meta?.dualSense ?? "";
  if (ds === "yes") return { short: "DualSense", long: "Auto: Steam lists DualSense support, so it starts directly." };
  if (g.padHint === "libScePad") return { short: "DualSense", long: "Auto: the game ships Sony's DualSense library, so it starts directly." };
  if (g.padHint === "SDL") return { short: "Native", long: "Auto: the game uses SDL, which handles a DualSense itself, so it starts directly." };
  if (ds === "dualshock") return { short: "DualShock", long: "Auto: Steam lists DualShock support, so it starts directly." };
  const r = routeOf(g, pad, tr);
  if (r === "bridge")
    return { short: "Xbox controller", long: "Auto: the game supports Xbox controllers but not PlayStation ones, so your controller works as an Xbox controller." };
  if (r === "steamInput")
    return { short: "Steam Input", long: "Auto: the game supports Xbox controllers but not PlayStation ones, so it starts through Steam Input." };
  const c = (g.meta?.controller ?? "").toLowerCase();
  if (c === "full" || c === "partial")
    return {
      short: "Auto",
      long: `Auto: the game supports Xbox controllers. With a PlayStation controller it starts ${tr.steam ? "through Steam Input" : "with Seaglass's Xbox controller, once it's set up in Settings"}.`,
    };
  return { short: "Auto", long: "Auto: no controller support known, so the game starts directly." };
}
