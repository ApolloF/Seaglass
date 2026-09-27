<script lang="ts">
  // Orbit layout: games as bubbles in a honeycomb that glides and magnifies
  // around the selected one. Only bubbles near the focus are rendered.
  import GameArt from "../components/GameArt.svelte";
  import GameTile from "../components/GameTile.svelte";
  import Icon from "../components/Icon.svelte";
  import Logo from "../components/Logo.svelte";
  import { metaLine, orbitOrder, padSummary } from "../lib/bp";
  import { bytes, playtime, ago } from "../lib/format";
  import { feedback, pad, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { lastPlayed, played, title, type Game } from "../lib/types";
  import Glyph from "./Glyph.svelte";
  import Hints from "./Hints.svelte";
  import { flat, hexDist, nextCell, place, spiral, type Dir } from "./orbit";
  import Sections, { type Section } from "./Sections.svelte";

  type Props = {
    width: number;
    height: number;
    onplay: (g: Game) => void;
    oninfo: (g: Game) => void;
    onfocus: (g: Game | null) => void;
    onlibrary: () => void;
    onsearch: () => void;
    onsettings: () => void;
    onfound: () => void;
    onmenu: () => void;
    ondesktop: () => void;
    onsection: (s: Section) => void;
    foundCount: number;
    checkCount: number;
  };
  let p: Props = $props();

  // Recently played games and favorites fill the middle.
  const games = $derived(orbitOrder(lib.base));
  const cells = $derived(spiral(games.length));

  let i = $state(0);
  let open = $state(false);
  let zoom = $state(1);
  const g = $derived(games[i] ?? null);
  $effect(() => p.onfocus(g));
  $effect(() => {
    if (i >= games.length) i = Math.max(0, games.length - 1);
  });

  const H = $derived(p.height);
  const CX = $derived(p.width / 2);
  const CY = $derived(H / 2 - 40);
  const B = 176; // a bubble at zoom 1
  // The element is as big as the selected bubble gets in the honeycomb,
  // and only scaled down there, so it's drawn sharp at every resolution
  // without every bubble being a large layer (at 4K, a hundred 560-pixel
  // bubbles were more than the GPU kept, and frames showed holes while
  // gliding). Opened, it's scaled up to OPEN and drawn again sharp once
  // it gets there.
  const D = 240;
  const OPEN = 560;
  // The honeycomb keeps clear of the top bar and the name capsule below.
  const lens = $derived({ ax: Math.min(880, p.width / 2 - 80), ay: H / 2 - 150, flat: 0.5 });

  const bubbles = $derived.by(() => {
    const fc = cells[i];
    if (!fc) return [];
    const size = B * zoom;
    const spacing = 198 * zoom;
    const focusSize = B * 1.25 * Math.max(zoom, 0.8);
    const f0 = flat(fc);
    const out = [];
    for (let k = 0; k < games.length; k++) {
      const c = cells[k];
      if (hexDist(c, fc) > (zoom < 1 ? 11 : 7)) continue;
      const f = k === i;
      const c0 = flat(c);
      const at = place(c0.x - f0.x, c0.y - f0.y, size, spacing, focusSize / size, lens);
      if (at.opacity <= 0) continue;
      let { x: X, y: Y, opacity: op } = at;
      let sc = (at.scale * size) / D;
      if (open) {
        // The chosen game opens up on the left; the rest fly out of view.
        if (f) ((X = -430), (Y = -20), (sc = OPEN / D));
        else ((X *= 1.9), (Y *= 1.9), (sc *= 1.3), (op = 0));
      }
      out.push({ g: games[k], k, f, tf: `translate(${X.toFixed(1)}px,${Y.toFixed(1)}px) scale(${sc.toFixed(3)})`, op, z: f ? 200 : Math.round(sc * 100), sc });
    }
    return out;
  });

  // Up and down keep to the column the last sideways move was in.
  let anchorX = $state(0);
  function select(k: number) {
    i = k;
    anchorX = flat(cells[k]).x;
  }
  function move(dir: Dir) {
    const k = nextCell(cells, i, dir, anchorX);
    if (k === null) return feedback.edge();
    i = k;
    if (dir === "left" || dir === "right") anchorX = flat(cells[k]).x;
    feedback.move();
  }

  // A held direction repeats quickly; the glide keeps up instead of lagging.
  let fast = $state(false);
  let fastTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(fastTimer));

  $effect(() =>
    useInput((intent, repeat) => {
      if (open) {
        if (intent === "confirm" && g) p.onplay(g);
        else if (intent === "back" || intent === "left") ((open = false), feedback.move());
        else if (intent === "info" && g) p.oninfo(g);
        else return false;
        return;
      }
      switch (intent) {
        case "left":
        case "right":
        case "up":
        case "down":
          fast = repeat;
          clearTimeout(fastTimer);
          if (repeat) fastTimer = setTimeout(() => (fast = false), 260);
          return move(intent);
        case "confirm":
          if (g?.needsReview) p.oninfo(g);
          else if (g) ((open = true), feedback.confirm());
          return;
        case "info":
          if (g) p.oninfo(g);
          return;
        case "lt":
        case "rt": {
          const z = intent === "lt" ? 0.62 : 1;
          if (z === zoom) return feedback.edge();
          zoom = z;
          feedback.move();
          return;
        }
      }
      return false;
    }),
  );

  const maxPlayed = $derived(Math.max(3600, ...games.map(played)));
  const maxSize = $derived(Math.max(1, ...games.map((x) => x.sizeBytes ?? 0)));
  const comps = $derived.by(() => {
    if (!g) return [];
    const ps = padSummary(g);
    return [
      { k: "Playtime", t: played(g) ? playtime(played(g)) : "New", val: played(g) ? `${Math.round(played(g) / 3600)}h` : "0h", pct: (played(g) / maxPlayed) * 100, color: "var(--accent-game)", icon: "" },
      { k: "Last played", t: ago(lastPlayed(g)), val: "", pct: lastPlayed(g) ? 100 : 8, color: "oklch(0.82 0.12 190)", icon: "clock" },
      { k: "Controller", t: ps.short, val: "", pct: ps.short === "Auto" ? 45 : 100, color: "oklch(0.78 0.12 255)", icon: "pad" },
      { k: "Size", t: g.sizeBytes ? bytes(g.sizeBytes) : "Unknown", val: "", pct: ((g.sizeBytes ?? 0) / maxSize) * 100, color: "oklch(0.85 0.16 135)", icon: "folder" },
    ] as const;
  });
  let logoFailed = $state(false);
  $effect(() => {
    g?.id;
    logoFailed = false;
  });
  // The selected game's backdrop fills the screen behind the honeycomb;
  // the last two stay mounted to crossfade.
  let backs = $state<Game[]>([]);
  let backTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const cur = g;
    clearTimeout(backTimer);
    if (!cur || backs[backs.length - 1]?.id === cur.id) return;
    // Not while flying past games with the stick held: only where it stops.
    backTimer = setTimeout(() => (backs = [...backs.filter((b) => b.id !== cur.id).slice(-1), cur]), fast ? 320 : 120);
    return () => clearTimeout(backTimer);
  });

  const clock = $state({ v: "" });
  $effect(() => {
    const tick = () => {
      const d = new Date();
      clock.v = `${String(d.getHours()).padStart(2, "0")}:${String(d.getMinutes()).padStart(2, "0")}`;
    };
    tick();
    const t = setInterval(tick, 15000);
    return () => clearInterval(t);
  });
</script>

<div class="orbit" class:fast class:opened={open}>
  <div class="backdrops">
    {#each backs as b (b.id)}
      <div class="backdrop" class:show={b.id === backs[backs.length - 1]?.id}><GameArt game={b} kind="backdrop" /></div>
    {/each}
  </div>
  <div class="veil"></div>
  <div class="glow" style:transform="translate({open ? CX - 430 : CX}px, {open ? CY - 20 : CY}px)"></div>

  <div class="top-left"><Logo size={30} /><Sections current="home" onpick={p.onsection} /></div>
  <div class="top-right">
    {#if pad.connected}<span class="status"><Icon name="pad" size={26} stroke={1.8} />{pad.battery >= 0 ? `${pad.battery}%` : ""}</span>{/if}
    <span class="clock">{clock.v}</span>
  </div>

  {#each bubbles as b (b.g.id)}
    <button
      type="button"
      class="bubble"
      style:left="{CX - D / 2}px"
      style:top="{CY - D / 2}px"
      style:width="{D}px"
      style:height="{D}px"
      style:transform={b.tf}
      style:opacity={b.op}
      style:z-index={b.z}
      class:f={b.f}
      onclick={() => (b.f ? (open ? p.onplay(b.g) : (open = true)) : (select(b.k), (open = false)))}
      aria-label={title(b.g)}
      tabindex="-1"
    >
      <GameTile game={b.g} big={b.f && open} />
    </button>
  {/each}

  {#if !open && g}
    <div class="capsule">
      <div class="ct">
        <span class="name">{title(g)}</span>
        <span class="sub">{g.sourceLabel} · {metaLine(g)}</span>
      </div>
      <button type="button" class="open" onclick={() => (open = true)}><span class="gl"><Glyph button="confirm" size={30} /></span>Open</button>
    </div>
    <div class="zoom">
      <span class="muted">Zoom</span>
      <button type="button" aria-label="Zoom out" onclick={() => (zoom = 0.62)}><span class="z">−</span></button>
      <button type="button" aria-label="Zoom in" onclick={() => (zoom = 1)}><span class="z">+</span></button>
    </div>
  {/if}

  {#if open && g}
    <div class="panel">
      <span class="src">{g.sourceLabel}</span>
      {#if g.meta?.logo && !logoFailed}
        <img class="logo" src={g.meta.logo} alt={title(g)} onerror={() => (logoFailed = true)} />
      {:else}
        <div class="title">{title(g)}</div>
      {/if}
      <div class="line">{metaLine(g)}</div>
      <div class="comps">
        {#each comps as c (c.k)}
          <div class="comp">
            <div class="ring" style:background="conic-gradient({c.color} 0 {c.pct}%, rgba(255,255,255,0.12) {c.pct}% 100%)">
              <div class="inner" style:color={c.color}>
                {#if c.icon === "clock"}<Icon name="clock" size={40} stroke={1.9} />
                {:else if c.icon === "pad"}<Icon name="pad" size={44} stroke={1.8} />
                {:else if c.icon === "folder"}<Icon name="folder" size={40} stroke={1.9} />
                {:else}<span class="val">{c.val}</span>{/if}
              </div>
            </div>
            <span class="t">{c.t}</span>
            <span class="k">{c.k}</span>
          </div>
        {/each}
      </div>
      <div class="buttons">
        <button type="button" class="play" onclick={() => p.onplay(g)}><span class="pic"><Icon name="play" size={22} /></span>Play</button>
        <button type="button" class="back" onclick={() => (open = false)}>Back</button>
      </div>
    </div>
  {/if}

  <div class="hints">
    <Hints
      hints={open
        ? [
            { button: "confirm", label: "Play" },
            { button: "info", label: "Details" },
            { button: "back", label: "Back" },
          ]
        : [
            { button: "confirm", label: g?.needsReview ? "Check" : "Open" },
            { button: "lt", also: "rt", label: "Zoom" },
            { button: "menu", label: "Quick access" },
          ]}
    />
  </div>
</div>

<style>
  .orbit {
    position: absolute;
    inset: 0;
    overflow: hidden;
    /* Its bubbles' z-indexes stay inside: the game page, quick access and
       the launch sequence open on top. */
    isolation: isolate;
    background: #000;
    color: #fff;
  }
  .backdrops,
  .backdrop {
    position: absolute;
    inset: 0;
  }
  .backdrop {
    opacity: 0;
    transition: opacity 0.8s ease;
  }
  .backdrop.show {
    opacity: 0.5;
  }
  .opened .backdrop.show {
    opacity: 0.75;
  }
  /* Darker in the middle, where the honeycomb is, and at the edges; when
     a game is open, dark on the right, behind its details. */
  .veil {
    position: absolute;
    inset: 0;
    background:
      radial-gradient(ellipse 60% 70% at 50% 46%, rgba(0, 0, 0, 0.55), rgba(0, 0, 0, 0.2) 70%),
      linear-gradient(180deg, rgba(0, 0, 0, 0.55) 0%, rgba(0, 0, 0, 0) 22%, rgba(0, 0, 0, 0) 70%, rgba(0, 0, 0, 0.8) 100%);
    transition: background 0.6s;
  }
  .opened .veil {
    background:
      linear-gradient(90deg, rgba(0, 0, 0, 0.15) 0%, rgba(0, 0, 0, 0.35) 40%, rgba(0, 0, 0, 0.88) 62%, rgba(0, 0, 0, 0.92) 100%),
      linear-gradient(180deg, rgba(0, 0, 0, 0.5) 0%, rgba(0, 0, 0, 0) 25%, rgba(0, 0, 0, 0) 75%, rgba(0, 0, 0, 0.7) 100%);
  }
  /* A soft light behind the selected game: a gradient, moved with a
     transform (a 220-pixel blur moved with left and top was repainted
     at full size every frame). */
  .glow {
    position: absolute;
    left: 0;
    top: 0;
    width: 1800px;
    height: 1400px;
    margin-left: -900px;
    margin-top: -700px;
    background: radial-gradient(closest-side, color-mix(in oklab, var(--accent-game) 32%, transparent), transparent);
    opacity: 0.7;
    transition: transform 0.7s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .top-left {
    position: absolute;
    left: 72px;
    top: 48px;
    display: flex;
    align-items: center;
    gap: 14px;
    font-size: 22px;
    font-weight: 700;
    z-index: 300;
  }
  .muted {
    color: rgba(255, 255, 255, 0.62);
    font-weight: 500;
  }
  .top-right {
    position: absolute;
    right: 72px;
    top: 40px;
    display: flex;
    align-items: center;
    gap: 24px;
    z-index: 300;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 18px;
    font-weight: 700;
    color: rgba(255, 255, 255, 0.8);
  }
  .clock {
    font-size: 42px;
    font-weight: 800;
    /* No colour transition: it can't run on the compositor, and a
       main-thread animation made every gliding bubble restyle each frame. */
    color: var(--accent-game);
  }
  .bubble {
    position: absolute;
    padding: 0;
    border: 0;
    border-radius: 50%;
    overflow: hidden;
    background: transparent;
    transform-origin: 50% 50%;
    /* In the bubble's own pixels: it scales with it. */
    box-shadow: 0 3px 10px rgba(0, 0, 0, 0.45);
    transition:
      transform 0.46s cubic-bezier(0.22, 1, 0.36, 1),
      opacity 0.4s ease;
  }
  /* Bubbles coming into view fade in with a CSS animation: Svelte's
     transitions tick from JavaScript every frame, and each such frame
     restyled every gliding bubble. */
  .bubble {
    animation: bubble-in 0.26s ease;
  }
  @keyframes bubble-in {
    from {
      opacity: 0;
    }
  }
  .bubble.f {
    box-shadow:
      0 0 0 4px #fff,
      0 0 30px color-mix(in oklab, var(--accent-game) 55%, transparent);
  }
  .opened .bubble.f {
    box-shadow: 0 0 14px color-mix(in oklab, var(--accent-game) 55%, transparent);
  }
  .fast .bubble {
    transition-duration: 0.2s, 0.2s;
    transition-timing-function: ease-out;
  }
  .capsule {
    position: absolute;
    left: 50%;
    bottom: 110px;
    transform: translateX(-50%);
    display: flex;
    align-items: center;
    gap: 26px;
    padding: 14px 16px 14px 32px;
    border-radius: 999px;
    /* Solid rather than a backdrop blur, which is redone every frame while
       the bubbles glide underneath. */
    background: rgba(24, 24, 28, 0.9);
    border: 1px solid rgba(255, 255, 255, 0.1);
    z-index: 280;
    white-space: nowrap;
    animation: fade 0.3s ease both;
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }
  .ct {
    display: flex;
    flex-direction: column;
    gap: 2px;
  }
  .name {
    font-size: 28px;
    font-weight: 800;
  }
  .sub {
    font-size: 18px;
    color: rgba(255, 255, 255, 0.7);
  }
  .open {
    height: 60px;
    padding: 0 26px 0 12px;
    border-radius: 30px;
    border: 0;
    background: #fff;
    color: #000;
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 21px;
    font-weight: 800;
  }
  .gl > :global(*) {
    background: #000;
    border-color: #000;
    color: #fff;
  }
  .zoom {
    position: absolute;
    right: 72px;
    bottom: 116px;
    display: flex;
    align-items: center;
    gap: 12px;
    z-index: 280;
  }
  .zoom button {
    width: 56px;
    height: 56px;
    border-radius: 50%;
    border: 1px solid rgba(255, 255, 255, 0.16);
    background: rgba(30, 30, 34, 0.78);
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .z {
    font-size: 30px;
    line-height: 1;
  }
  .panel {
    position: absolute;
    left: 1000px;
    top: 0;
    bottom: 0;
    width: 820px;
    display: flex;
    flex-direction: column;
    justify-content: center;
    gap: 28px;
    z-index: 280;
    animation: panel 0.55s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  }
  @keyframes panel {
    from {
      opacity: 0;
      transform: translateX(40px);
    }
  }
  .src {
    font-size: 18px;
    font-weight: 800;
    letter-spacing: 0.12em;
    text-transform: uppercase;
    color: var(--accent-game);
  }
  .logo {
    max-width: 640px;
    max-height: 200px;
    object-fit: contain;
    object-position: left center;
  }
  .title {
    font-family: var(--font-display);
    font-size: 88px;
    font-weight: 700;
    line-height: 0.95;
    text-wrap: balance;
  }
  .line {
    font-size: 21px;
    color: rgba(255, 255, 255, 0.74);
  }
  .comps {
    display: flex;
    gap: 22px;
  }
  .comp {
    width: 150px;
    display: flex;
    flex-direction: column;
    align-items: center;
    gap: 10px;
  }
  .ring {
    position: relative;
    width: 118px;
    height: 118px;
    border-radius: 50%;
  }
  .inner {
    position: absolute;
    inset: 9px;
    border-radius: 50%;
    background: #0b0b0d;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .val {
    font-size: 30px;
    font-weight: 800;
  }
  .t {
    font-size: 18px;
    font-weight: 800;
    text-align: center;
  }
  .k {
    font-size: 15px;
    color: rgba(255, 255, 255, 0.62);
  }
  .buttons {
    display: flex;
    gap: 14px;
  }
  .play {
    height: 80px;
    padding: 0 44px 0 14px;
    border-radius: 40px;
    border: 0;
    background: #fff;
    color: #000;
    display: flex;
    align-items: center;
    gap: 16px;
    font-size: 28px;
    font-weight: 800;
  }
  .pic {
    width: 54px;
    height: 54px;
    border-radius: 50%;
    background: #000;
    color: #fff;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .back {
    height: 80px;
    padding: 0 32px;
    border-radius: 40px;
    border: 1px solid rgba(255, 255, 255, 0.18);
    background: rgba(255, 255, 255, 0.07);
    color: #fff;
    font-size: 22px;
    font-weight: 700;
  }
  .hints {
    position: absolute;
    right: 72px;
    left: 72px;
    bottom: 40px;
    z-index: 300;
  }
</style>
