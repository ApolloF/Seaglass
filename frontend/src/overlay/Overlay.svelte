<script lang="ts">
  // The in-game overlay: a borderless window on top of the game, opened by
  // the PS / Xbox button. It only reads the controller; nothing is injected
  // into the game.
  import GameArt from "../components/GameArt.svelte";
  import Hints from "../bigpicture/Hints.svelte";
  import Stage from "../bigpicture/Stage.svelte";
  import { clamp } from "../bigpicture/nav";
  import { api } from "../lib/api";
  import { clock, playtime } from "../lib/format";
  import { dispatch, feedback, keyIntent, pad, useInput, type Intent } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import { played, title } from "../lib/types";

  lib
    .init()
    .then(async () => Object.assign(pad, await api.pad.state()))
    // Without the library the overlay has nothing to show: it goes, and
    // the PS button brings it back to try again.
    .catch(() => close());
  // The battery and connection change while the overlay is open.
  $effect(() => api.pad.onState((st) => Object.assign(pad, st)));

  const s = $derived(lib.session);
  const game = $derived(s ? (lib.games.find((g) => g.id === s.gameId) ?? null) : null);
  const accent = $derived(game?.meta?.accent ?? "oklch(0.8 0.12 205)");

  let i = $state(0);
  let quitArmed = $state(false);
  // One disarm timer: an older one would cut a fresh "press again" short.
  let quitTimer: ReturnType<typeof setTimeout> | undefined;
  $effect(() => () => clearTimeout(quitTimer));
  let closing = false;

  function close() {
    if (closing) return;
    closing = true;
    api.launch.closeOverlay();
  }

  // The game ended (or was quit): the overlay has nothing left to do.
  $effect(() => {
    if (s && s.phase !== "running" && lib.loaded) close();
  });

  async function quit() {
    if (!quitArmed) {
      quitArmed = true;
      clearTimeout(quitTimer);
      quitTimer = setTimeout(() => (quitArmed = false), 4000);
      return;
    }
    try {
      await api.launch.quitGame();
    } catch {
      feedback.error();
    }
  }

  const items = $derived([
    { id: "resume", title: "Back to the game", detail: "Close this overlay", run: close },
    { id: "library", title: "Open Seaglass", detail: "The game keeps running", run: () => api.launch.openMain() },
    {
      id: "quit",
      title: quitArmed ? "Press again to quit" : "Quit game",
      detail: quitArmed ? "Unsaved progress is lost" : "Ends the game right away",
      run: quit,
    },
  ]);

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "up":
        case "down": {
          const j = clamp(i + (intent === "up" ? -1 : 1), 0, items.length - 1);
          if (j !== i) {
            i = j;
            quitArmed = false;
            clearTimeout(quitTimer);
            feedback.move();
          }
          return;
        }
        case "confirm":
          items[i].run();
          return;
        case "back":
        case "home":
        case "menu":
          close();
          return;
      }
    }),
  );

  $effect(() => api.launch.onOverlayAction((a, repeat) => dispatch(a as Intent, repeat)));

  function onkeydown(e: KeyboardEvent) {
    const it = keyIntent(e);
    if (!it) return;
    e.preventDefault();
    dispatch(it, e.repeat);
  }

  // Always dark, like big picture.
  document.documentElement.dataset.theme = "dark";
</script>

<svelte:window {onkeydown} />

<Stage clear>
  {#snippet children({ height })}
    <div class="overlay" style:--accent-game={accent} style:height="{height}px">
      <button type="button" class="scrim" aria-label="Back to the game" onclick={close}></button>
      <aside class="panel">
        {#if game}
          <div class="art"><GameArt {game} kind="hero" /></div>
        {/if}
        <div class="head">
          <span class="label">Playing</span>
          <h1>{game ? title(game) : (s?.title ?? "")}</h1>
          <div class="stats">
            <div><span class="v">{clock(s?.seconds ?? 0)}</span><span class="k">this session</span></div>
            {#if game}<div><span class="v">{playtime(played(game))}</span><span class="k">in total</span></div>{/if}
            {#if pad.connected}<div><span class="v">{pad.battery >= 0 ? `${pad.battery}%` : "—"}</span><span class="k">controller</span></div>{/if}
          </div>
        </div>
        <ul class="menu">
          {#each items as it, j (it.id)}
            <li>
              <button type="button" class:on={j === i} class:danger={it.id === "quit" && quitArmed} onclick={() => ((i = j), it.run())} onmouseenter={() => (i = j)}>
                <span class="t">{it.title}</span>
                <span class="d">{it.detail}</span>
              </button>
            </li>
          {/each}
        </ul>
        <div class="hints"><Hints hints={[{ button: "confirm", label: "Select" }, { button: "back", label: "Back to game" }]} /></div>
      </aside>
    </div>
  {/snippet}
</Stage>

<style>
  .overlay {
    position: absolute;
    inset: 0;
    color: #e8edf2;
    font-family: var(--font);
    user-select: none;
    animation: fade 0.2s ease both;
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }
  .scrim {
    position: absolute;
    inset: 0;
    border: 0;
    background: rgba(3, 5, 8, 0.62);
    cursor: default;
  }
  .panel {
    position: absolute;
    left: 0;
    top: 0;
    bottom: 0;
    width: 700px;
    background: rgba(10, 14, 19, 0.96);
    border-right: 1px solid rgba(255, 255, 255, 0.08);
    display: flex;
    flex-direction: column;
    animation: slide 0.28s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  }
  @keyframes slide {
    from {
      transform: translateX(-40px);
      opacity: 0;
    }
  }
  .art {
    position: absolute;
    inset: 0 0 auto 0;
    height: 420px;
    opacity: 0.35;
    mask-image: linear-gradient(180deg, #000 30%, transparent);
  }
  .head {
    position: relative;
    padding: 110px 64px 40px;
    display: flex;
    flex-direction: column;
    gap: 16px;
  }
  .label {
    font-size: 18px;
    font-weight: 700;
    letter-spacing: 0.14em;
    text-transform: uppercase;
    color: var(--accent-game);
  }
  h1 {
    margin: 0;
    font-family: var(--font-display);
    font-size: 64px;
    line-height: 1;
  }
  .stats {
    display: flex;
    gap: 44px;
    margin-top: 10px;
  }
  .stats div {
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .v {
    font-family: var(--font-display);
    font-size: 40px;
    font-variant-numeric: tabular-nums;
  }
  .k {
    font-size: 17px;
    color: #8795a3;
  }
  .menu {
    list-style: none;
    margin: 0;
    padding: 0 40px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .menu button {
    width: 100%;
    text-align: left;
    padding: 20px 24px;
    border-radius: 16px;
    border: 2px solid transparent;
    background: rgba(255, 255, 255, 0.04);
    color: inherit;
    font-family: inherit;
    display: flex;
    flex-direction: column;
    gap: 4px;
    transition: background 0.15s ease;
  }
  .menu button.on {
    border-color: var(--accent-game);
    background: rgba(255, 255, 255, 0.09);
  }
  .menu button.danger.on {
    border-color: #ff8f8f;
  }
  .t {
    font-size: 26px;
    font-weight: 600;
  }
  .danger .t {
    color: #ffb3b3;
  }
  .d {
    font-size: 18px;
    color: #8795a3;
  }
  .hints {
    margin-top: auto;
    padding: 0 64px 48px;
  }
  @media (prefers-reduced-motion: reduce) {
    .overlay,
    .panel {
      animation: none;
    }
  }
</style>
