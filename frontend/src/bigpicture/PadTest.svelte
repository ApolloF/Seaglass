<script lang="ts">
  // The controller test: every button lights up as it's pressed, the
  // sticks and triggers move as they do, so a worn stick or a button that
  // doesn't register shows. Every button is being tested, so leaving is
  // holding ○ / B (Esc on a keyboard); holding ✕ / A plays each rumble.
  import { api } from "../lib/api";
  import { input, pad, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import type { PadRaw } from "../lib/types";
  import Hints from "./Hints.svelte";

  let { onback }: { onback: () => void } = $props();

  let raw = $state<PadRaw>({ buttons: 0, axes: [0, 0, 0, 0, 0, 0] });
  let seen = $state(0); // every button pressed since the screen opened
  let last = $state("");

  const ps = $derived(pad.kind === "playstation");
  // SDL's gamepad buttons.
  const B = { south: 0, east: 1, west: 2, north: 3, back: 4, guide: 5, start: 6, ls: 7, rs: 8, lb: 9, rb: 10, up: 11, down: 12, left: 13, right: 14, misc: 15, touch: 20 };
  const names = $derived<Record<number, string>>(
    ps
      ? { 0: "✕", 1: "○", 2: "□", 3: "△", 4: "Create", 5: "PS", 6: "Options", 7: "L3", 8: "R3", 9: "L1", 10: "R1", 11: "Up", 12: "Down", 13: "Left", 14: "Right", 15: "Mute", 20: "Touchpad" }
      : { 0: "A", 1: "B", 2: "X", 3: "Y", 4: "View", 5: "Xbox", 6: "Menu", 7: "LS", 8: "RS", 9: "LB", 10: "RB", 11: "Up", 12: "Down", 13: "Left", 14: "Right", 15: "Share", 20: "Touchpad" },
  );
  const down = (b: number) => (raw.buttons & (1 << b)) !== 0;
  const was = (b: number) => (seen & (1 << b)) !== 0;
  const cls = (b: number) => (down(b) ? "b down" : was(b) ? "b seen" : "b");
  const ax = (k: number) => (raw.axes[k] ?? 0) / 32768;
  const trig = (k: number) => Math.max(0, raw.axes[k] ?? 0) / 32767;

  // Holding ○ leaves; holding ✕ plays the rumble effects.
  let holdStart = $state<Record<number, number>>({});
  let now = $state(performance.now());
  const HOLD = 1200;
  const progress = (b: number) => (holdStart[b] ? Math.min(1, (now - holdStart[b]) / HOLD) : 0);

  $effect(() => {
    api.pad.testInput(true);
    const off = api.pad.onRaw((r) => {
      const pressed = r.buttons & ~raw.buttons;
      for (let b = 0; b < 32; b++) if (pressed & (1 << b)) last = names[b] ?? `Button ${b}`;
      seen |= r.buttons;
      const hs = { ...holdStart };
      for (const b of [B.south, B.east]) {
        if (r.buttons & (1 << b)) hs[b] ??= performance.now();
        else delete hs[b];
      }
      holdStart = hs;
      raw = r;
    });
    // A clock only while a button is held, for the hold rings.
    let frame = 0;
    const tick = () => {
      if (!holdStart[B.east] && !holdStart[B.south]) {
        frame = requestAnimationFrame(tick);
        return;
      }
      now = performance.now();
      if (progress(B.east) >= 1) {
        onback();
        return;
      }
      if (progress(B.south) >= 1) {
        rumbleAll();
        holdStart = { ...holdStart, [B.south]: Infinity };
      }
      frame = requestAnimationFrame(tick);
    };
    frame = requestAnimationFrame(tick);
    return () => {
      cancelAnimationFrame(frame);
      off();
      api.pad.testInput(false);
    };
  });

  let rumbling = $state("");
  async function rumbleAll() {
    if (!lib.settings?.haptics) {
      rumbling = "Haptics are off in Settings";
      return;
    }
    for (const e of ["tick", "bump", "confirm", "error", "launch"] as const) {
      rumbling = e;
      api.pad.rumble(e);
      await new Promise((r) => setTimeout(r, 700));
    }
    rumbling = "";
  }

  // Actions still arrive from the controller: this screen takes them all.
  // The keyboard's Esc leaves at once.
  $effect(() =>
    useInput((intent) => {
      if (intent === "back" && input.source === "keyboard") onback();
    }),
  );

  const stickDot = (x: number, y: number) => `translate(${(x * 44).toFixed(1)}px, ${(y * 44).toFixed(1)}px)`;
  const allButtons = [0, 1, 2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14];
  const tested = $derived(allButtons.filter(was).length);
</script>

<div class="test">
  <div class="head">
    <h1>Test controller</h1>
    <span class="who">{pad.connected ? `${pad.name}${pad.wireless ? " · Bluetooth" : " · USB"}${pad.battery >= 0 ? ` · ${pad.battery} %` : ""}` : "No controller connected"}</span>
  </div>

  <svg class="pad" viewBox="0 0 1100 640" aria-label="Controller">
    <!-- body -->
    <path class="body" d="M300 120 H800 C920 120 1000 180 1030 300 L1075 480 C1095 580 990 640 915 560 L840 470 H260 L185 560 C110 640 5 580 25 480 L70 300 C100 180 180 120 300 120 Z" />
    <!-- triggers and shoulders -->
    <g>
      <rect class="trig" x="150" y="20" width="200" height="26" rx="8" />
      <rect class="fill" x="150" y="20" width={200 * trig(4)} height="26" rx="8" />
      <text x="250" y="39">{ps ? "L2" : "LT"} {Math.round(trig(4) * 100)} %</text>
      <rect class="trig" x="750" y="20" width="200" height="26" rx="8" />
      <rect class="fill" x="750" y="20" width={200 * trig(5)} height="26" rx="8" />
      <text x="850" y="39">{ps ? "R2" : "RT"} {Math.round(trig(5) * 100)} %</text>
      <rect class={cls(B.lb)} x="150" y="62" width="200" height="34" rx="14" /><text x="250" y="85">{names[B.lb]}</text>
      <rect class={cls(B.rb)} x="750" y="62" width="200" height="34" rx="14" /><text x="850" y="85">{names[B.rb]}</text>
    </g>
    <!-- d-pad -->
    <g>
      <rect class={cls(B.up)} x="228" y="200" width="44" height="54" rx="8" />
      <rect class={cls(B.down)} x="228" y="296" width="44" height="54" rx="8" />
      <rect class={cls(B.left)} x="166" y="253" width="54" height="44" rx="8" />
      <rect class={cls(B.right)} x="280" y="253" width="54" height="44" rx="8" />
    </g>
    <!-- face buttons -->
    <g class="face">
      <circle class={cls(B.north)} cx="850" cy="205" r="32" /><text x="850" y="216">{names[B.north]}</text>
      <circle class={cls(B.east)} cx="918" cy="275" r="32" /><text x="918" y="286">{names[B.east]}</text>
      <circle class={cls(B.south)} cx="850" cy="345" r="32" /><text x="850" y="356">{names[B.south]}</text>
      <circle class={cls(B.west)} cx="782" cy="275" r="32" /><text x="782" y="286">{names[B.west]}</text>
    </g>
    <!-- middle: touchpad (PlayStation), small buttons, guide -->
    {#if ps}
      <rect class={cls(B.touch)} x="425" y="140" width="250" height="140" rx="22" /><text x="550" y="218">Touchpad</text>
    {/if}
    <rect class={cls(B.back)} x={ps ? 372 : 440} y={ps ? 150 : 240} width="34" height="52" rx="12" />
    <text class="small" x={ps ? 389 : 457} y={ps ? 224 : 314}>{names[B.back]}</text>
    <rect class={cls(B.start)} x={ps ? 694 : 626} y={ps ? 150 : 240} width="34" height="52" rx="12" />
    <text class="small" x={ps ? 711 : 643} y={ps ? 224 : 314}>{names[B.start]}</text>
    <circle class={cls(B.guide)} cx="550" cy={ps ? 330 : 175} r="30" /><text x="550" y={ps ? 341 : 186}>{ps ? "PS" : "⊗"}</text>
    <rect class={cls(B.misc)} x="530" y={ps ? 378 : 330} width="40" height="16" rx="8" />
    <!-- sticks -->
    <g>
      <circle class="well" cx="390" cy="420" r="70" />
      <circle class={cls(B.ls)} cx="390" cy="420" r="38" style:transform={stickDot(ax(0), ax(1))} style:transform-origin="390px 420px" />
      <circle class="well" cx="710" cy="420" r="70" />
      <circle class={cls(B.rs)} cx="710" cy="420" r="38" style:transform={stickDot(ax(2), ax(3))} style:transform-origin="710px 420px" />
    </g>
  </svg>

  <div class="side">
    <div class="stat"><span class="k">Last pressed</span><span class="v">{last || "—"}</span></div>
    <div class="stat"><span class="k">Buttons that worked</span><span class="v">{tested} of {allButtons.length}</span></div>
    <div class="stat">
      <span class="k">Left stick</span><span class="v mono">{ax(0).toFixed(2)}, {ax(1).toFixed(2)}</span>
    </div>
    <div class="stat">
      <span class="k">Right stick</span><span class="v mono">{ax(2).toFixed(2)}, {ax(3).toFixed(2)}</span>
    </div>
    <p class="help">A stick that shows movement while you don't touch it drifts. Hold {names[B.south]} to feel each rumble{rumbling ? `: ${rumbling}` : ""}.</p>
    <div class="hold">
      <span class="ring" style:--p={progress(B.east)}></span>
      Hold {names[B.east]} to leave
    </div>
  </div>

  <div class="hints">
    <Hints hints={[{ button: "back", label: input.source === "keyboard" ? "Leave" : "Hold to leave" }]} />
  </div>
</div>

<style>
  .test {
    position: absolute;
    inset: 0;
    background: radial-gradient(ellipse at 40% 45%, #111925, #06080b 70%);
    color: #e8edf2;
    padding: 56px 110px;
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 28px;
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 52px;
  }
  .who {
    font-size: 22px;
    color: rgba(232, 237, 242, 0.7);
  }
  .pad {
    position: absolute;
    left: 110px;
    top: 190px;
    width: 1100px;
    height: 640px;
    overflow: visible;
  }
  .body {
    fill: #161d27;
    stroke: rgba(255, 255, 255, 0.1);
    stroke-width: 2;
  }
  .b,
  .trig,
  .well {
    fill: #222c38;
    stroke: rgba(255, 255, 255, 0.14);
    stroke-width: 2;
  }
  .well {
    fill: #0f141b;
  }
  .b.seen {
    fill: #25413f;
    stroke: oklch(0.7 0.1 175);
  }
  .b.down {
    fill: oklch(0.82 0.13 190);
    stroke: #fff;
  }
  .fill {
    fill: oklch(0.82 0.13 190);
  }
  text {
    fill: #e8edf2;
    font-size: 24px;
    font-weight: 700;
    text-anchor: middle;
    pointer-events: none;
  }
  text.small {
    font-size: 17px;
    font-weight: 600;
    fill: rgba(232, 237, 242, 0.7);
  }
  .side {
    position: absolute;
    right: 110px;
    top: 200px;
    width: 460px;
    display: flex;
    flex-direction: column;
    gap: 22px;
  }
  .stat {
    display: flex;
    flex-direction: column;
    gap: 4px;
    padding: 18px 22px;
    border-radius: 18px;
    background: rgba(255, 255, 255, 0.05);
  }
  .k {
    font-size: 17px;
    color: rgba(232, 237, 242, 0.62);
    font-weight: 600;
  }
  .v {
    font-size: 30px;
    font-weight: 700;
  }
  .mono {
    font-variant-numeric: tabular-nums;
  }
  .help {
    margin: 0;
    font-size: 19px;
    line-height: 1.4;
    color: rgba(232, 237, 242, 0.72);
  }
  .hold {
    display: flex;
    align-items: center;
    gap: 14px;
    font-size: 21px;
    font-weight: 600;
  }
  .ring {
    width: 34px;
    height: 34px;
    border-radius: 50%;
    background: conic-gradient(oklch(0.82 0.13 190) calc(var(--p) * 100%), rgba(255, 255, 255, 0.12) 0);
    mask: radial-gradient(circle, transparent 55%, #000 57%);
  }
  .hints {
    position: absolute;
    right: 110px;
    bottom: 48px;
  }
</style>
