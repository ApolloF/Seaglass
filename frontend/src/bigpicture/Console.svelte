<script lang="ts">
  // Console layout: one row of games over full-screen art. Down shows the
  // selected game's details.
  import { untrack } from "svelte";
  import GameArt from "../components/GameArt.svelte";
  import Icon from "../components/Icon.svelte";
  import Logo from "../components/Logo.svelte";
  import { api } from "../lib/api";
  import { metaLine, padSummary, recentFirst } from "../lib/bp";
  import { bytes } from "../lib/format";
  import { feedback, pad, useInput, type Intent } from "../lib/input.svelte";
  import { savesSummary } from "../lib/saves";
  import { isFresh, lib } from "../lib/store.svelte";
  import { title, type Game, type Saves } from "../lib/types";
  import Hints from "./Hints.svelte";
  import { rowOffset } from "./nav";
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

  const games = $derived(recentFirst(lib.base).slice(0, 40));
  let i = $state(0);
  let details = $state(false);
  const n = $derived(games.length + 1); // + "All games"
  const g = $derived(games[i] ?? null);
  // The list re-sorts (a game just played moves to the front): the cursor
  // stays on the game, not on the slot. Re-find runs before the id is
  // recorded again, so it still sees the old one.
  let selId: number | null = null;
  $effect(() => {
    const list = games;
    const k = selId === null ? -1 : list.findIndex((x) => x.id === selId);
    if (k >= 0) untrack(() => k !== i && (i = k));
  });
  $effect(() => {
    selId = g?.id ?? null;
  });
  $effect(() => p.onfocus(g));
  $effect(() => {
    if (i >= n) i = Math.max(0, n - 1);
  });

  // Keep the last two heroes mounted so they can crossfade. Not while
  // flying past games with the stick held: each one is a full-screen
  // picture to decode, so only where it stops (as in Orbit).
  let heroes = $state<Game[]>([]);
  let heroTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => {
    const cur = g;
    clearTimeout(heroTimer);
    if (!cur || heroes[heroes.length - 1]?.id === cur.id) return;
    const show = () => (heroes = [...heroes.filter((h) => h.id !== cur.id).slice(-1), cur]);
    if (heroes.length === 0) show();
    else heroTimer = setTimeout(show, fast ? 320 : 90);
    return () => clearTimeout(heroTimer);
  });

  // Details: the stick moves between the buttons and the cards, a card
  // opens in place, and L1 / R1 change the game.
  type Focus = "play" | "fav" | "hide" | Card;
  type Card = "about" | "pad" | "copy";
  const rowA: Focus[] = ["play", "fav", "hide"];
  const rowB: Card[] = ["about", "pad", "copy"];
  let focus = $state<Focus>("play");
  let lastCard = $state<Card>("about");
  let open = $state<Card | null>(null);
  const modes = ["", "native", "steam"] as const;
  const modeInfo = [
    { label: "Auto", text: "Seaglass decides from what the game supports." },
    { label: "Native", text: "Always starts directly. The game handles the controller itself." },
    { label: "Steam Input", text: "Always starts through Steam Input, so a DualSense acts as an Xbox controller." },
  ];
  let pick = $state(0);
  const choosable = $derived(!!g && !g.launchUri);
  // The game can go away underneath (uninstalled, list refreshed).
  $effect(() => {
    if (details && !g) ((details = false), (open = null));
  });

  function showDetails(on: boolean) {
    details = on;
    focus = "play";
    open = null;
    feedback.move();
  }
  function switchGame(d: -1 | 1) {
    const j = i + d;
    if (j < 0 || j >= games.length) return feedback.edge();
    i = j;
    open = null;
    feedback.move();
  }
  function press(f: Focus) {
    if (!g) return;
    focus = f;
    if (f === "play") return p.onplay(g);
    if (f === "hide") return showDetails(false);
    feedback.confirm();
    if (f === "fav") {
      const game = g;
      lib.run(() => api.setFavorite(game.id, !game.favorite));
    } else {
      lastCard = f;
      pick = Math.max(0, modes.indexOf((g.padMode ?? "") as (typeof modes)[number]));
      open = f;
    }
  }
  function choose(k: number) {
    if (!g) return;
    const game = g;
    if (modes[k] !== (game.padMode ?? "")) lib.run(() => api.setPadMode(game.id, modes[k]));
    feedback.confirm();
    open = null;
  }

  function detailsInput(intent: Intent): boolean | void {
    if (intent === "lb" || intent === "rb") return switchGame(intent === "lb" ? -1 : 1);
    if (open) {
      if (intent === "back" || intent === "info") ((open = null), feedback.move());
      else if (intent === "confirm") open === "pad" && choosable ? choose(pick) : ((open = null), feedback.move());
      else if (open === "pad" && choosable && (intent === "left" || intent === "right")) {
        const k = Math.max(0, Math.min(modes.length - 1, pick + (intent === "left" ? -1 : 1)));
        if (k !== pick) ((pick = k), feedback.move());
        else feedback.edge();
      } else if (intent === "menu" || intent === "home" || intent === "view") return false;
      return;
    }
    const row: Focus[] = rowB.includes(focus as Card) ? rowB : rowA;
    const at = row.indexOf(focus);
    switch (intent) {
      case "left":
      case "right": {
        const k = at + (intent === "left" ? -1 : 1);
        if (k >= 0 && k < row.length) ((focus = row[k]), feedback.move());
        else feedback.edge();
        return;
      }
      case "down":
        if (row === rowA) ((focus = lastCard), feedback.move());
        else feedback.edge();
        return;
      case "up":
        if (row === rowB) {
          lastCard = focus as Card;
          focus = "play";
          feedback.move();
        } else showDetails(false);
        return;
      case "confirm":
        return press(focus);
      case "back":
      case "info":
        return showDetails(false);
    }
    return false;
  }

  // A held direction: tiles change size at once rather than animating
  // their width and height, which lays out and repaints the row every
  // frame (the glide of the row itself stays).
  let fast = $state(false);
  let fastTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(fastTimer));
  function held(repeat: boolean) {
    fast = repeat;
    clearTimeout(fastTimer);
    if (repeat) fastTimer = setTimeout(() => (fast = false), 260);
  }

  $effect(() =>
    useInput((intent, repeat) => {
      if (details) return detailsInput(intent);
      switch (intent) {
        case "left":
          held(repeat);
          if (i > 0) ((i -= 1), feedback.move());
          else feedback.edge();
          return;
        case "right":
          held(repeat);
          if (i < n - 1) ((i += 1), feedback.move());
          else feedback.edge();
          return;
        case "up":
          feedback.edge();
          return;
        case "down":
        case "info":
          if (g) showDetails(true);
          return;
        case "confirm":
          if (g?.needsReview) p.oninfo(g);
          else if (g) p.onplay(g);
          else (feedback.confirm(), p.onlibrary());
          return;
      }
      return false;
    }),
  );

  let saves = $state<Saves | null>(null);
  const savesInfo = $derived(savesSummary(saves));
  $effect(() => {
    const game = open === "copy" ? g : null;
    saves = null;
    if (!game?.installed) return;
    let live = true;
    api.saves
      .get(game.id)
      .then((s) => live && (saves = s))
      .catch(() => {});
    return () => (live = false);
  });

  const TILE = 172;
  const GAP = 20;
  const rowX = $derived(rowOffset(i, TILE, GAP));
  const logoSize = $derived(g ? Math.round(Math.min(130, 1500 / (title(g).length * 0.6))) : 100);
  let logoFailed = $state(false);
  $effect(() => {
    g?.id;
    logoFailed = false;
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

<div class="console" class:fast>
  <div class="heroes">
    {#each heroes as h (h.id)}
      <div class="hero" class:show={h.id === heroes[heroes.length - 1]?.id}><GameArt game={h} kind="backdrop" /></div>
    {/each}
  </div>
  <div class="scrim-l"></div>
  <div class="scrim-v"></div>
  <div class="dim" class:on={details}></div>

  <div class="top">
    <Logo size={32} />
    <span class="gap"></span>
    <Sections current="home" onpick={p.onsection} />
    <div class="grow"></div>
    {#if pad.connected}<span class="status"><Icon name="pad" size={26} stroke={1.8} />{pad.battery >= 0 ? `${pad.battery}%` : ""}</span>{/if}
    <span class="clock">{clock.v}</span>
  </div>

  <div class="slide" style:transform="translateY({details ? -300 : 0}px)">
    <div class="tiles" class:hide={details}>
      <div class="strip" style:transform="translateX({rowX}px)">
        {#each games as t, k (t.id)}
          {@const on = k === i}
          <button type="button" class="tile" class:on style:width="{on ? 400 : TILE}px" style:height="{on ? 225 : TILE}px" onclick={() => (on ? p.onplay(t) : (i = k))} aria-label={title(t)}>
            <GameArt game={t} kind={on ? "hero" : "cover"} />
            {#if on}<span class="tname">{title(t)}</span>{/if}
            {#if isFresh(t)}<span class="new">NEW</span>{/if}
          </button>
        {/each}
        <button type="button" class="tile all" class:on={i === games.length} style:width="{TILE}px" style:height="{TILE}px" onclick={p.onlibrary}>
          <Icon name="grid" size={36} stroke={1.7} />
          <span>All games</span>
        </button>
      </div>
    </div>

    {#if g}
      <div class="info" class:away={open}>
        <span class="pill">{g.sourceLabel}</span>
        {#if g.meta?.logo && !logoFailed}
          <img class="logo" src={g.meta.logo} alt={title(g)} onerror={() => (logoFailed = true)} />
        {:else}
          <div class="title" style:font-size="{logoSize}px">{title(g)}</div>
        {/if}
        <div class="line">{metaLine(g)}{g.meta?.genres?.length ? ` · ${g.meta.genres.slice(0, 2).join(", ")}` : ""}</div>
        <div class="chips">
          <span class="chip"><Icon name="pad" size={22} stroke={1.8} />{padSummary(g).short}</span>
          {#if g.favorite}<span class="chip"><Icon name="star" size={20} />Favorite</span>{/if}
          {#if g.needsReview}<span class="chip warn"><Icon name="warn" size={20} />Needs a check</span>{/if}
        </div>
        <div class="buttons">
          <button type="button" class="play" class:on={details && focus === "play"} onclick={() => press("play")}>
            <span class="pic"><Icon name="play" size={22} /></span>
            Play
          </button>
          {#if details}
            <button type="button" class="round" class:on={focus === "fav"} class:fav={g.favorite} aria-label={g.favorite ? "Remove from favorites" : "Add to favorites"} onclick={() => press("fav")}>
              <Icon name="star" size={30} stroke={1.8} />
            </button>
          {/if}
          <button type="button" class="round" class:on={details && focus === "hide"} aria-label={details ? "Hide details" : "Details"} onclick={() => (details ? press("hide") : showDetails(true))}>
            <Icon name={details ? "chevronDown" : "info"} size={30} stroke={1.8} />
          </button>
        </div>
      </div>

      <div class="cards" class:show={details} class:away={open}>
        <button type="button" class="card" class:on={details && focus === "about"} tabindex={details ? 0 : -1} onclick={() => press("about")}>
          <span class="ch"><Icon name="info" size={22} stroke={1.8} /><span>About</span></span>
          <span class="big">{g.meta?.developers?.[0] ?? g.sourceLabel}{g.meta?.releaseYear ? ` · ${g.meta.releaseYear}` : ""}</span>
          <span class="p">{g.meta?.description ?? "No store description for this game."}</span>
        </button>
        <button type="button" class="card" class:on={details && focus === "pad"} tabindex={details ? 0 : -1} onclick={() => press("pad")}>
          <span class="ch"><Icon name="pad" size={24} stroke={1.8} /><span>Controller</span></span>
          <span class="big">{padSummary(g).short}</span>
          <span class="p">{padSummary(g).long}</span>
        </button>
        <button type="button" class="card" class:on={details && focus === "copy"} tabindex={details ? 0 : -1} onclick={() => press("copy")}>
          <span class="ch"><Icon name="scan" size={22} stroke={1.8} /><span>This copy</span></span>
          <span class="dl">
            <span class="dt">Found</span>
            <span>{g.how}</span>
            <span class="dt">Identified</span>
            <span>{g.matchHow}</span>
            {#if g.sizeBytes}<span class="dt">Size</span><span>{bytes(g.sizeBytes)}</span>{/if}
          </span>
        </button>
      </div>

      {#if open}
        <div class="expanded" role="dialog" aria-label={open === "about" ? "About" : open === "pad" ? "Controller" : "This copy"}>
          <div class="eh">
            {#if open === "about"}<Icon name="info" size={24} stroke={1.8} /><span>About</span>
            {:else if open === "pad"}<Icon name="pad" size={26} stroke={1.8} /><span>Controller</span>
            {:else}<Icon name="scan" size={24} stroke={1.8} /><span>This copy</span>{/if}
            <span class="grow"></span>
            <button type="button" class="x" aria-label="Close" onclick={() => (open = null)}><Icon name="close" size={24} /></button>
          </div>
          {#if open === "about"}
            <div class="big">{g.meta?.developers?.join(", ") || g.sourceLabel}{g.meta?.releaseYear ? ` · ${g.meta.releaseYear}` : ""}</div>
            <p class="full">{g.meta?.description ?? "No store description for this game."}</p>
            <dl>
              {#if g.meta?.genres?.length}<dt>Genres</dt><dd>{g.meta.genres.join(", ")}</dd>{/if}
              {#if g.meta?.publishers?.length}<dt>Publisher</dt><dd>{g.meta.publishers.join(", ")}</dd>{/if}
              {#if g.meta?.releaseDate}<dt>Released</dt><dd>{g.meta.releaseDate}</dd>{/if}
              <dt>Played</dt><dd>{metaLine(g)}</dd>
            </dl>
          {:else if open === "pad"}
            <p class="full">{padSummary(g).long}</p>
            {#if choosable}
              <div class="modes">
                {#each modeInfo as m, k (m.label)}
                  <button type="button" class="mode" class:on={pick === k} class:current={modes[k] === (g.padMode ?? "")} onclick={() => choose(k)}>
                    <span class="ml">{m.label}{#if modes[k] === (g.padMode ?? "")}<span class="cur">Current</span>{/if}</span>
                    <span class="mt">{m.text}</span>
                  </button>
                {/each}
              </div>
            {/if}
          {:else}
            <dl>
              <dt>Found</dt><dd>{g.how}</dd>
              <dt>Identified</dt><dd>{g.matchHow}</dd>
              <dt>Source</dt><dd>{g.sourceLabel}</dd>
              {#if g.sizeBytes}<dt>Size</dt><dd>{bytes(g.sizeBytes)}</dd>{/if}
              {#if g.dir}<dt>Folder</dt><dd class="path">{g.dir}</dd>{/if}
              {#if g.exe}<dt>Program</dt><dd class="path">{g.exe}</dd>{/if}
              {#if savesInfo && saves?.installed}<dt>Saves</dt><dd class:warn={savesInfo.tone === "warn"}>{savesInfo.text}</dd>{/if}
            </dl>
          {/if}
        </div>
      {/if}
    {/if}
  </div>

  <div class="hints">
    <Hints
      hints={details
        ? [
            ...(open && !(open === "pad" && choosable)
              ? []
              : [
                  {
                    button: "confirm" as const,
                    label: open ? "Choose" : focus === "play" ? "Play" : focus === "fav" ? (g?.favorite ? "Remove favorite" : "Add to favorites") : focus === "hide" ? "Hide details" : "Open",
                  },
                ]),
            { button: "lb", also: "rb", label: "Switch game" },
            { button: "back", label: open ? "Close" : "Back" },
          ]
        : [
            { button: "confirm", label: g ? (g.needsReview ? "Check" : "Play") : "Open" },
            { button: "info", label: "Details" },
            { button: "menu", label: "Quick access" },
          ]}
    />
  </div>
</div>

<style>
  .console {
    position: absolute;
    inset: 0;
    overflow: hidden;
    isolation: isolate;
    background: #06080b;
    color: #f3f5f7;
  }
  .heroes {
    position: absolute;
    inset: 0;
  }
  .hero {
    position: absolute;
    inset: 0;
    opacity: 0;
    transition: opacity 0.7s ease;
  }
  .hero.show {
    opacity: 1;
    animation: kb 16s ease-out both;
  }
  @keyframes kb {
    from {
      transform: scale(1.07);
    }
  }
  .scrim-l {
    position: absolute;
    inset: 0;
    background: linear-gradient(90deg, rgba(6, 8, 11, 0.94) 0%, rgba(6, 8, 11, 0.62) 36%, rgba(6, 8, 11, 0) 68%);
  }
  .scrim-v {
    position: absolute;
    inset: 0;
    background: linear-gradient(180deg, rgba(6, 8, 11, 0.82) 0%, rgba(6, 8, 11, 0) 30%, rgba(6, 8, 11, 0) 60%, rgba(6, 8, 11, 0.92) 100%);
  }
  .dim {
    position: absolute;
    inset: 0;
    background: rgba(6, 8, 11, 0.72);
    opacity: 0;
    transition: opacity 0.5s;
  }
  .dim.on {
    opacity: 1;
  }
  .top {
    position: absolute;
    left: 96px;
    right: 96px;
    top: 34px;
    height: 60px;
    display: flex;
    align-items: center;
    gap: 10px;
    z-index: 5;
  }
  .gap {
    width: 18px;
  }
  .grow {
    flex: 1;
  }
  .status {
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 18px;
    font-weight: 600;
    margin-right: 20px;
  }
  .clock {
    font-size: 24px;
    font-weight: 700;
  }
  .slide {
    position: absolute;
    inset: 0;
    transition: transform 0.6s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .tiles {
    position: absolute;
    left: 96px;
    right: 0;
    top: 128px;
    height: 240px;
    transition: opacity 0.5s;
  }
  .tiles.hide {
    opacity: 0;
  }
  .strip {
    display: flex;
    gap: 20px;
    align-items: flex-start;
    transition: transform 0.5s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .tile {
    position: relative;
    flex-shrink: 0;
    padding: 0;
    border: 0;
    border-radius: 22px;
    overflow: hidden;
    background: #11151b;
    box-shadow: 0 12px 30px rgba(0, 0, 0, 0.45);
    transition:
      width 0.45s cubic-bezier(0.2, 0.8, 0.2, 1),
      height 0.45s cubic-bezier(0.2, 0.8, 0.2, 1),
      box-shadow 0.3s;
  }
  .fast .tile {
    transition: none;
  }
  .tile.on {
    box-shadow:
      0 0 0 4px #f3f5f7,
      0 20px 50px rgba(0, 0, 0, 0.55),
      0 0 70px color-mix(in oklab, var(--accent-game) 45%, transparent);
  }
  .tname {
    position: absolute;
    left: 20px;
    right: 20px;
    bottom: 16px;
    text-align: left;
    font-family: var(--font-display);
    font-weight: 700;
    font-size: 30px;
    color: #fff;
    text-shadow: 0 2px 16px rgba(0, 0, 0, 0.7);
  }
  .new {
    position: absolute;
    top: 12px;
    right: 12px;
    padding: 4px 10px;
    border-radius: 999px;
    background: #f3f5f7;
    color: #06080b;
    font-size: 13px;
    font-weight: 800;
    letter-spacing: 0.06em;
  }
  .tile.all {
    border: 1px solid rgba(255, 255, 255, 0.14);
    background: rgba(18, 22, 28, 0.6);
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    color: #f3f5f7;
    font-size: 18px;
    font-weight: 700;
  }
  .info {
    position: absolute;
    left: 96px;
    top: 436px;
    width: 1040px;
    display: flex;
    flex-direction: column;
    gap: 22px;
    transition: opacity 0.3s;
  }
  .info.away {
    opacity: 0;
  }
  .pill {
    width: fit-content;
    padding: 7px 14px;
    border-radius: 999px;
    border: 1px solid rgba(255, 255, 255, 0.24);
    font-size: 15px;
    font-weight: 700;
    letter-spacing: 0.08em;
    text-transform: uppercase;
  }
  .logo {
    max-width: 800px;
    max-height: 230px;
    object-fit: contain;
    object-position: left bottom;
    filter: drop-shadow(0 6px 30px rgba(0, 0, 0, 0.55));
  }
  .title {
    font-family: var(--font-display);
    font-weight: 700;
    line-height: 0.95;
    max-width: 1000px;
    text-wrap: balance;
    text-shadow: 0 4px 30px rgba(0, 0, 0, 0.45);
  }
  .line {
    font-size: 22px;
    color: rgba(243, 245, 247, 0.82);
  }
  .chips {
    display: flex;
    gap: 12px;
  }
  .chip {
    display: inline-flex;
    align-items: center;
    gap: 10px;
    height: 44px;
    padding: 0 18px;
    border-radius: 999px;
    background: rgba(14, 18, 24, 0.62);
    border: 1px solid rgba(255, 255, 255, 0.1);
    font-size: 18px;
    font-weight: 600;
  }
  .chip.warn {
    color: #ffd28a;
  }
  .buttons {
    display: flex;
    gap: 16px;
    margin-top: 6px;
  }
  .play {
    height: 80px;
    padding: 0 46px 0 14px;
    border-radius: 40px;
    border: 0;
    background: #f3f5f7;
    color: #06080b;
    display: flex;
    align-items: center;
    gap: 18px;
    font-size: 28px;
    font-weight: 800;
    box-shadow: 0 14px 44px color-mix(in oklab, var(--accent-game) 45%, transparent);
  }
  .pic {
    width: 54px;
    height: 54px;
    border-radius: 50%;
    background: #06080b;
    color: #f3f5f7;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .round {
    width: 80px;
    height: 80px;
    border-radius: 50%;
    border: 1px solid rgba(255, 255, 255, 0.2);
    background: rgba(14, 18, 24, 0.55);
    color: #f3f5f7;
    display: flex;
    align-items: center;
    justify-content: center;
    transition: box-shadow 0.2s;
  }
  .round.fav {
    color: #ffd28a;
  }
  .round.fav :global(svg) {
    fill: currentColor;
  }
  .play.on,
  .round.on {
    box-shadow:
      0 0 0 4px #06080b,
      0 0 0 7px #f3f5f7;
  }
  .cards {
    position: absolute;
    left: 96px;
    right: 96px;
    top: 950px;
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 24px;
    opacity: 0;
    transition: opacity 0.5s;
  }
  .cards.show {
    opacity: 1;
  }
  .cards.show.away {
    opacity: 0;
    transition: opacity 0.25s;
  }
  .card {
    height: 300px;
    padding: 28px;
    border-radius: 28px;
    background: rgba(14, 18, 24, 0.74);
    border: 1px solid rgba(255, 255, 255, 0.09);
    color: inherit;
    font: inherit;
    text-align: left;
    display: flex;
    flex-direction: column;
    gap: 10px;
    overflow: hidden;
    transition:
      box-shadow 0.2s,
      transform 0.3s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .card.on {
    box-shadow:
      0 0 0 4px #f3f5f7,
      0 20px 50px rgba(0, 0, 0, 0.45);
    transform: translateY(-4px);
  }
  .ch {
    display: flex;
    align-items: center;
    gap: 10px;
    font-size: 18px;
    font-weight: 700;
    color: rgba(243, 245, 247, 0.78);
  }
  .big {
    font-size: 30px;
    font-weight: 800;
  }
  .p {
    font-size: 18px;
    line-height: 1.45;
    color: rgba(243, 245, 247, 0.78);
    display: -webkit-box;
    -webkit-line-clamp: 5;
    line-clamp: 5;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  dl,
  .dl {
    display: grid;
    grid-template-columns: 120px minmax(0, 1fr);
    gap: 9px 12px;
    margin: 0;
    font-size: 17px;
  }
  dt,
  .dt {
    color: rgba(243, 245, 247, 0.6);
  }
  dd {
    margin: 0;
  }
  dd.warn {
    color: #ffd28a;
  }
  .path {
    overflow-wrap: anywhere;
  }
  /* A card opened in place: it grows over the details it came from. */
  .expanded {
    position: absolute;
    left: 96px;
    right: 96px;
    /* Bottom-aligned with the cards (top 950px + 300px), growing upwards. */
    bottom: calc(100% - 1250px);
    min-height: 300px;
    max-height: 640px;
    padding: 34px 40px;
    border-radius: 32px;
    background: rgba(14, 18, 24, 0.9);
    border: 1px solid rgba(255, 255, 255, 0.12);
    box-shadow: 0 30px 80px rgba(0, 0, 0, 0.5);
    backdrop-filter: blur(24px);
    display: flex;
    flex-direction: column;
    gap: 18px;
    overflow: hidden;
    transform-origin: 50% 100%;
    animation: grow 0.35s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  }
  @keyframes grow {
    from {
      opacity: 0;
      transform: translateY(40px) scale(0.97);
    }
  }
  .eh {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 22px;
    font-weight: 700;
    color: rgba(243, 245, 247, 0.85);
  }
  .x {
    width: 48px;
    height: 48px;
    border-radius: 50%;
    border: 1px solid rgba(255, 255, 255, 0.16);
    background: transparent;
    color: #f3f5f7;
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .full {
    margin: 0;
    max-width: 1300px;
    font-size: 22px;
    line-height: 1.5;
    color: rgba(243, 245, 247, 0.84);
    display: -webkit-box;
    -webkit-line-clamp: 8;
    line-clamp: 8;
    -webkit-box-orient: vertical;
    overflow: hidden;
  }
  .expanded dl {
    grid-template-columns: 170px minmax(0, 1fr);
    gap: 12px 18px;
    font-size: 20px;
  }
  .modes {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 20px;
    margin-top: 8px;
  }
  .mode {
    min-height: 170px;
    padding: 24px 26px;
    border-radius: 24px;
    border: 1px solid rgba(255, 255, 255, 0.14);
    background: rgba(255, 255, 255, 0.05);
    color: #f3f5f7;
    text-align: left;
    display: flex;
    flex-direction: column;
    gap: 10px;
    transition:
      box-shadow 0.2s,
      background 0.2s;
  }
  .mode.current {
    background: color-mix(in oklab, var(--accent-game) 16%, rgba(255, 255, 255, 0.05));
  }
  .mode.on {
    box-shadow:
      0 0 0 4px #06080b,
      0 0 0 7px #f3f5f7;
  }
  .ml {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 26px;
    font-weight: 800;
  }
  .cur {
    padding: 3px 10px;
    border-radius: 999px;
    background: #f3f5f7;
    color: #06080b;
    font-size: 13px;
    font-weight: 800;
    letter-spacing: 0.06em;
    text-transform: uppercase;
  }
  .mt {
    font-size: 18px;
    line-height: 1.45;
    color: rgba(243, 245, 247, 0.75);
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 40px;
    left: 96px;
    z-index: 5;
  }
</style>
