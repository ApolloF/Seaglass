<script lang="ts">
  import { accentOf, newFinds } from "../lib/bp";
  import { dispatchFrom, feedback, input, keyIntent, setBase, setLight, toHex } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import type { Game } from "../lib/types";
  import BPSettings from "./BPSettings.svelte";
  import Console from "./Console.svelte";
  import Deck from "./Deck.svelte";
  import GameSheet from "./GameSheet.svelte";
  import Launching from "./Launching.svelte";
  import LibraryScreen from "./LibraryScreen.svelte";
  import Orbit from "./Orbit.svelte";
  import PadTest from "./PadTest.svelte";
  import QuickAccess from "./QuickAccess.svelte";
  import SearchScreen from "./SearchScreen.svelte";
  import { SECTIONS, type Section } from "./Sections.svelte";
  import Stage from "./Stage.svelte";

  let { onexit }: { onexit: () => void } = $props();

  type Screen = Section | "settings" | "found" | "padtest";
  let screen = $state<Screen>("home");
  let qa = $state(false);
  let sheetId = $state<number | null>(null);
  // The launch sequence shows for the current session until put away.
  let hiddenSession = $state(-1);
  let focused = $state<Game | null>(null);

  const layout = $derived(lib.settings?.bigPictureLayout ?? "deck");
  const sheet = $derived(sheetId === null ? null : (lib.games.find((g) => g.id === sheetId) ?? null));
  const session = $derived(lib.session);
  // Only games started from big picture get the launch sequence; one
  // started elsewhere shows it only when it asks something or fails.
  const showLaunch = $derived(
    !!session &&
      session.id !== hiddenSession &&
      session.phase !== "" &&
      session.phase !== "cancelled" &&
      (session.from === "bigpicture" || !!session.question || session.phase === "failed") &&
      (session.phase !== "ended" || session.startedAt || session.note),
  );
  const launchGame = $derived(session ? (lib.games.find((g) => g.id === session.gameId) ?? null) : null);
  // A session that ended before this window opened (the interface was
  // closed while playing) shows its summary once; older ones don't.
  let initial = true;
  $effect(() => {
    if (!initial || !session) return;
    initial = false;
    if (session.phase === "ended" && session.startedAt && Date.now() / 1000 - session.startedAt - session.seconds > 60) hiddenSession = session.id;
  });
  // New on this PC: found in the last week, or matched by folder name only.
  const found = $derived(newFinds(lib.base, 500));
  // The accent colour follows the selected game once the selection rests:
  // it's a custom property on the root, and changing one restyles every
  // element on screen, which with a direction held (a new game every
  // tenth of a second) cost more than a frame each time.
  const wanted = $derived(accentOf(focused) ?? "oklch(0.8 0.12 205)");
  let accent = $state("oklch(0.8 0.12 205)");
  $effect(() => {
    const next = wanted;
    const t = setTimeout(() => (accent = next), 160);
    return () => clearTimeout(t);
  });
  const lightHex = $derived(toHex(accent));

  $effect(() => setLight(accent));

  const go = (s: Screen) => {
    if (s === screen && !qa) return;
    screen = s;
    qa = false;
    feedback.move();
  };
  const play = (g: Game) => {
    if (!g.installed) {
      // Owned but not installed: its page offers the store's install.
      if (g.installUri) sheetId = g.id;
      else feedback.error();
      return;
    }
    sheetId = null;
    feedback.launch();
    lib.play(g);
  };
  const info = (g: Game) => {
    sheetId = g.id;
    feedback.move();
  };
  const onfocus = (g: Game | null) => (focused = g);

  // L1 / R1 step through the sections from anywhere; settings and New on
  // this PC sit next to Home.
  const section = $derived<Section | null>(screen === "home" || screen === "library" || screen === "search" ? screen : null);
  function stepSection(d: -1 | 1) {
    const at = SECTIONS.findIndex((s) => s.id === (section ?? "home"));
    const next = SECTIONS[at + d];
    if (next) go(next.id);
    else if (section === null) go("home");
    else feedback.edge();
  }

  // Under every screen: the buttons that work everywhere.
  $effect(() => {
    setBase((i) => {
      if (i === "menu" || i === "home") {
        qa = !qa;
        feedback.move();
      } else if (i === "view") go("search");
      else if (i === "lb" || i === "rb") stepSection(i === "lb" ? -1 : 1);
      else if (i === "back" && screen !== "home") go("home");
      else if (i === "back" && input.source === "keyboard") {
        // Esc at home: Quick access has the way back to desktop mode.
        qa = true;
        feedback.move();
      }
    });
    return () => setBase(null);
  });

  // Big picture is always dark.
  $effect(() => {
    const prev = document.documentElement.dataset.theme;
    document.documentElement.dataset.theme = "dark";
    return () => {
      if (prev) document.documentElement.dataset.theme = prev;
    };
  });

  // The pointer hides while the controller or keyboard is in use, and after
  // a few seconds without the mouse moving.
  let pointerTimer: ReturnType<typeof setTimeout> | undefined;
  function onpointermove(e: PointerEvent) {
    if (e.pointerType !== "mouse" || (e.movementX === 0 && e.movementY === 0)) return;
    input.pointer = true;
    clearTimeout(pointerTimer);
    pointerTimer = setTimeout(() => (input.pointer = false), 3000);
  }
  $effect(() => () => clearTimeout(pointerTimer));

  function onkeydown(e: KeyboardEvent) {
    const i = keyIntent(e);
    if (!i) return;
    e.preventDefault();
    dispatchFrom("keyboard", i, e.repeat);
  }

  const layoutProps = $derived({
    onplay: play,
    oninfo: info,
    onfocus,
    onlibrary: () => go("library"),
    onsearch: () => go("search"),
    onsettings: () => go("settings"),
    onfound: () => go("found"),
    onmenu: () => (qa = true),
    ondesktop: onexit,
    onsection: (s: Section) => go(s),
    foundCount: found.length,
    checkCount: found.filter((g) => g.needsReview).length,
  });
</script>

<svelte:window {onkeydown} {onpointermove} />

<Stage>
  {#snippet children({ width, height })}
    <div class="bp" class:nopointer={!input.pointer} style:--accent-game={accent}>
      {#if screen === "home"}
        {#if layout === "console"}
          <Console {width} {height} {...layoutProps} />
        {:else if layout === "orbit"}
          <Orbit {width} {height} {...layoutProps} />
        {:else}
          <Deck {width} {height} {...layoutProps} />
        {/if}
      {:else if screen === "library"}
        <LibraryScreen {width} {height} onplay={play} oninfo={info} {onfocus} onback={() => go("home")} onsection={(s) => go(s)} />
      {:else if screen === "found"}
        <LibraryScreen {width} {height} games={found} review onplay={play} oninfo={info} {onfocus} onback={() => go("home")} onsection={(s) => go(s)} />
      {:else if screen === "padtest"}
        <PadTest onback={() => go("settings")} />
      {:else if screen === "search"}
        <SearchScreen {width} active={!sheet && !qa && !showLaunch} onplay={play} oninfo={info} {onfocus} onback={() => go("home")} onsection={(s) => go(s)} />
      {:else}
        <BPSettings onback={() => go("home")} onpadtest={() => go("padtest")} />
      {/if}

      {#if sheet}
        <GameSheet game={sheet} onplay={() => play(sheet)} onclose={() => (sheetId = null)} />
      {/if}
      {#if qa}
        <QuickAccess light={lightHex} onclose={() => (qa = false)} onsettings={() => go("settings")} ondesktop={onexit} />
      {/if}
      {#if showLaunch && session}
        <Launching {session} game={launchGame} onclose={() => (hiddenSession = session.id)} />
      {/if}
    </div>
  {/snippet}
</Stage>

<style>
  .bp {
    position: absolute;
    inset: 0;
    background: #0a0e13;
    color: #e8edf2;
    font-family: var(--font);
    overflow: hidden;
    user-select: none;
  }
  .bp :global(button) {
    font-family: inherit;
    cursor: pointer;
  }
  .bp.nopointer,
  .bp.nopointer :global(*) {
    cursor: none !important;
  }
  .bp :global(button:focus-visible) {
    outline: none;
  }
</style>
