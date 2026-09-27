// Big picture input: controller actions (from the Go side) and keyboard keys
// become one set of intents, delivered to the top-most screen that wants
// them. Screens register with useInput() inside an $effect, so an overlay
// opened on top gets input first and gives it back when it closes.
import { api } from "./api";
import { lib } from "./store.svelte";
import type { PadState } from "./types";

export type Intent =
  | "up"
  | "down"
  | "left"
  | "right"
  | "confirm"
  | "back"
  | "action"
  | "info"
  | "menu"
  | "view"
  | "home"
  | "lb"
  | "rb"
  | "lt"
  | "rt";

/** A handler returns false to let the screen below handle the intent. */
export type Handler = (intent: Intent, repeat: boolean) => boolean | void;

const stack: Handler[] = [];

export function useInput(h: Handler): () => void {
  stack.push(h);
  return () => {
    const i = stack.lastIndexOf(h);
    if (i >= 0) stack.splice(i, 1);
  };
}

let base: Handler | null = null;

/** The handler under every screen (menu, home), set by the big picture shell. */
export function setBase(h: Handler | null) {
  base = h;
}

export function dispatch(intent: Intent, repeat = false) {
  for (let i = stack.length - 1; i >= 0; i--) {
    if (stack[i](intent, repeat) !== false) return;
  }
  base?.(intent, repeat);
}

/** Where input last came from: it picks the button prompts. The mouse
 * pointer shows only while the mouse is being used. */
export const input = $state<{ source: "pad" | "keyboard"; pointer: boolean }>({ source: "pad", pointer: false });

// Steam's desktop configuration turns a controller into a keyboard while
// Seaglass reads the same controller, so one press can arrive twice:
// once from the controller and once as a key. The second copy is dropped.
const TWIN_MS = 120;
const lastSeen = new Map<Intent, { from: "pad" | "keyboard"; at: number }>();
let lastPad = 0;

/** Delivers input from a source, unless it is the other source's copy of the same press. */
export function dispatchFrom(from: "pad" | "keyboard", intent: Intent, repeat = false): boolean {
  const now = performance.now();
  const prev = lastSeen.get(intent);
  if (prev && prev.from !== from && now - prev.at < TWIN_MS) return false;
  // Held: keys auto-repeat faster than the controller does; its own repeat wins.
  if (from === "keyboard" && repeat && now - lastPad < 400) return false;
  lastSeen.set(intent, { from, at: now });
  if (from === "pad") lastPad = now;
  if (input.source !== from) input.source = from;
  if (input.pointer) input.pointer = false;
  dispatch(intent, repeat);
  return true;
}

const KEYS: Record<string, Intent> = {
  ArrowUp: "up",
  ArrowDown: "down",
  ArrowLeft: "left",
  ArrowRight: "right",
  Enter: "confirm",
  " ": "confirm",
  Escape: "back",
  Backspace: "back",
  x: "action",
  X: "action",
  i: "info",
  I: "info",
  y: "info",
  Y: "info",
  m: "menu",
  M: "menu",
  Tab: "menu",
  f: "view",
  F: "view",
  "/": "view",
  q: "lb",
  Q: "lb",
  e: "rb",
  E: "rb",
  PageUp: "lb",
  PageDown: "rb",
  "[": "lt",
  "]": "rt",
  "-": "lt",
  "=": "rt",
  "+": "rt",
  Home: "home",
};

/** The keys shown in prompts while the keyboard is in use. */
export const KEY_LABELS: Record<Intent, string> = {
  up: "↑",
  down: "↓",
  left: "←",
  right: "→",
  confirm: "Enter",
  back: "Esc",
  action: "X",
  info: "I",
  menu: "M",
  view: "F",
  home: "Home",
  lb: "Q",
  rb: "E",
  lt: "[",
  rt: "]",
};

// Keys that still work while typing in a field: they don't edit text, so
// the sections (PageUp / PageDown) and Quick access (Tab) stay in reach.
const FIELD_KEYS = new Set(["Escape", "ArrowDown", "ArrowUp", "PageUp", "PageDown", "Tab"]);

/** Maps a key to an intent, unless the user is typing in a field. */
export function keyIntent(e: KeyboardEvent): Intent | null {
  const t = e.target as HTMLElement | null;
  if (t && (t.tagName === "INPUT" || t.tagName === "TEXTAREA") && !FIELD_KEYS.has(e.key)) return null;
  if (e.ctrlKey || e.altKey || e.metaKey) return null;
  return KEYS[e.key] ?? null;
}

// ---- controller state and feedback ----

export const pad = $state<PadState>({ connected: false, name: "", kind: "other", dualSense: false, battery: -1, wireless: false });

/** Which prompts to show: keys while the keyboard is in use (or no
 * controller is connected), else the user's choice, else the controller's. */
export function glyphSet(): "playstation" | "xbox" | "keyboard" {
  if (input.source === "keyboard" || !pad.connected) return "keyboard";
  const g = lib.settings?.glyphs ?? "auto";
  if (g === "playstation" || g === "xbox") return g;
  return pad.kind === "playstation" ? "playstation" : "xbox";
}

let audio: AudioContext | null = null;
function beep(freq: number, ms: number, gain = 0.035) {
  if (!lib.settings?.sounds) return;
  try {
    audio ??= new AudioContext();
    const o = audio.createOscillator();
    const g = audio.createGain();
    o.type = "sine";
    o.frequency.value = freq;
    g.gain.setValueAtTime(gain, audio.currentTime);
    g.gain.exponentialRampToValueAtTime(0.0001, audio.currentTime + ms / 1000);
    o.connect(g).connect(audio.destination);
    o.start();
    o.stop(audio.currentTime + ms / 1000);
  } catch {
    /* no audio device */
  }
}

/** Feedback for moving focus, reaching an edge, confirming and errors: rumble and sound. */
export const feedback = {
  move() {
    api.pad.rumble("tick");
    beep(1250, 30);
  },
  /** Nothing further that way. */
  edge() {
    api.pad.rumble("bump");
    beep(330, 45, 0.03);
  },
  confirm() {
    api.pad.rumble("confirm");
    beep(880, 60, 0.05);
    setTimeout(() => beep(1320, 70, 0.04), 55);
  },
  error() {
    api.pad.rumble("error");
    beep(220, 140, 0.05);
  },
  launch() {
    api.pad.rumble("launch");
    beep(660, 80, 0.04);
    setTimeout(() => beep(990, 120, 0.04), 90);
  },
};

/** Moves focus if `next` differs, with a tick, or bumps at the edge. Returns whether it moved. */
export function moveTo<T>(cur: T, next: T | null | undefined, set: (v: T) => void): boolean {
  if (next === null || next === undefined || next === cur) {
    feedback.edge();
    return false;
  }
  set(next);
  feedback.move();
  return true;
}

let lastLight = "";
/** Tints the DualSense lightbar (the Go side checks the user's setting). */
export function setLight(css: string | undefined) {
  const hex = css ? toHex(css) : "";
  if (!hex || hex === lastLight) return;
  lastLight = hex;
  api.pad.setLight(hex);
}

let ctx: CanvasRenderingContext2D | null = null;
/** Any CSS colour (oklch too) as #rrggbb. */
export function toHex(css: string): string {
  if (/^#[0-9a-f]{6}$/i.test(css)) return css.toLowerCase();
  ctx ??= document.createElement("canvas").getContext("2d", { willReadFrequently: true });
  if (!ctx) return "";
  ctx.clearRect(0, 0, 1, 1);
  ctx.fillStyle = "#000";
  ctx.fillStyle = css;
  ctx.fillRect(0, 0, 1, 1);
  const [r, g, b] = ctx.getImageData(0, 0, 1, 1).data;
  return "#" + [r, g, b].map((v) => v.toString(16).padStart(2, "0")).join("");
}
