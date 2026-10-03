<script lang="ts">
  import BigPicture from "./bigpicture/BigPicture.svelte";
  import Toasts from "./components/Toasts.svelte";
  import Welcome from "./components/Welcome.svelte";
  import Desktop from "./desktop/Desktop.svelte";
  import { api } from "./lib/api";
  import { desktopPad } from "./lib/desknav";
  import { dispatchFrom, pad, type Intent } from "./lib/input.svelte";
  import { errText, lib } from "./lib/store.svelte";
  import { sessionActive } from "./lib/types";

  let failed = $state("");
  const welcome = $derived(!!lib.settings && !lib.settings.welcomed);
  let mode = $state<"desktop" | "bigpicture">("desktop");

  function enterBigPicture() {
    if (mode === "bigpicture") return;
    mode = "bigpicture";
    api.window.fullscreen(true);
    api.launch.setUIMode(mode);
  }
  function exitBigPicture() {
    if (mode === "desktop") return;
    mode = "desktop";
    api.window.fullscreen(false);
    api.launch.setUIMode(mode);
  }

  // The window comes back after a game in the mode the game was started
  // from; only a first start follows the setting.
  const resume = new URLSearchParams(location.search).get("mode");

  lib
    .init()
    .then(async () => {
      Object.assign(pad, await api.pad.state());
      if (resume === "bigpicture" || (!resume && lib.settings?.startInBigPicture)) enterBigPicture();
    })
    .catch((e) => (failed = errText(e)));

  // Around a game the controller layer lets go of the controller and takes
  // it back, which looks like a controller connecting: that mustn't switch
  // to big picture. Neither while a game runs, nor just after it (or just
  // after the window came back for it).
  let padQuietUntil = resume ? performance.now() + 15000 : 0;
  let lastPhase = "";
  $effect(() => {
    const phase = lib.session?.phase ?? "";
    if (phase !== lastPhase && (phase === "ended" || phase === "failed")) padQuietUntil = performance.now() + 15000;
    lastPhase = phase;
  });

  $effect(() => api.launch.onUIMode((m) => (m === "bigpicture" ? enterBigPicture() : exitBigPicture())));

  // Controller: in big picture every action goes to the screens; in desktop
  // mode it moves focus around the window, and the PS / Xbox button
  // switches to big picture.
  $effect(() =>
    api.pad.onAction((action, repeat) => {
      if (mode === "bigpicture") dispatchFrom("pad", action as Intent, repeat);
      else if (action === "home" && !repeat && !welcome) enterBigPicture();
      else if (!failed) desktopPad(action as Intent, repeat);
    }),
  );
  $effect(() =>
    api.pad.onState((s) => {
      const wasConnected = pad.connected;
      Object.assign(pad, s);
      // Not during the welcome: big picture's screens would take the
      // controller from it.
      if (s.connected && !wasConnected && !welcome && lib.settings?.openBigPictureOnController && !sessionActive(lib.session) && performance.now() > padQuietUntil) enterBigPicture();
    }),
  );

  // Styles outside a mode (toasts) know which one shows.
  $effect(() => {
    document.documentElement.dataset.mode = mode;
  });

  // Desktop mode follows the Windows theme unless the user picked one.
  let systemDark = $state(window.matchMedia("(prefers-color-scheme: dark)").matches);
  $effect(() => {
    const mq = window.matchMedia("(prefers-color-scheme: dark)");
    const on = (e: MediaQueryListEvent) => (systemDark = e.matches);
    mq.addEventListener("change", on);
    return () => mq.removeEventListener("change", on);
  });
  $effect(() => {
    if (mode === "bigpicture") return;
    const pref = lib.settings?.theme ?? "system";
    const dark = pref === "dark" || (pref === "system" && systemDark);
    document.documentElement.dataset.theme = dark ? "dark" : "light";
  });

  function onkeydown(e: KeyboardEvent) {
    if (e.key === "F11") {
      e.preventDefault();
      if (mode === "bigpicture") exitBigPicture();
      else enterBigPicture();
    }
  }

  // No right-click browser menu, except in text fields.
  function oncontextmenu(e: MouseEvent) {
    const t = e.target as HTMLElement;
    if (!t.closest("input, textarea")) e.preventDefault();
  }
</script>

<svelte:window {oncontextmenu} {onkeydown} />

{#if failed}
  <div class="fatal">
    <h1>Seaglass couldn't start</h1>
    <p>{failed}</p>
  </div>
{:else}
  <!-- Behind the first-start welcome, nothing takes focus. -->
  <div class="mode" inert={welcome}>
    {#if mode === "bigpicture"}
      <BigPicture onexit={exitBigPicture} />
    {:else}
      <Desktop onbigpicture={enterBigPicture} />
    {/if}
  </div>
  {#if welcome}<Welcome {mode} onbigpicture={enterBigPicture} />{/if}
{/if}
<Toasts />

<style>
  .mode {
    display: contents;
  }
  .fatal {
    height: 100%;
    display: flex;
    flex-direction: column;
    align-items: center;
    justify-content: center;
    gap: 8px;
    padding: 40px;
    text-align: center;
  }
  .fatal h1 {
    margin: 0;
    font-family: var(--font-display);
  }
  .fatal p {
    margin: 0;
    color: var(--muted);
    user-select: text;
  }
</style>
