<script lang="ts">
  // The first start: what Seaglass does, where it looks for games,
  // and how you'd like to play. It works with a mouse, the keyboard or a
  // controller, in either mode. Skipping it counts as seen.
  import { api } from "../lib/api";
  import { feedback, input, pad, useInput } from "../lib/input.svelte";
  import { lib } from "../lib/store.svelte";
  import type { Settings } from "../lib/types";
  import Icon from "./Icon.svelte";

  let { mode, onbigpicture }: { mode: "desktop" | "bigpicture"; onbigpicture: () => void } = $props();

  let step = $state(0);
  const steps = ["Welcome", "Your games", "How you play", "Ready"];
  let autoFolders = $state<string[]>([]);
  $effect(() => {
    api.autoFolders().then((f) => (autoFolders = f)).catch(() => {});
  });
  const s = $derived(lib.settings);
  const games = $derived(lib.counts.all);

  function save(patch: Partial<Settings>) {
    if (s) lib.saveSettings({ ...s, ...patch });
  }
  async function addFolder() {
    try {
      lib.settings = await api.addFolder();
      api.rescan();
    } catch {
      /* the picker was closed */
    }
  }
  function finish(big: boolean) {
    save({ welcomed: true });
    feedback.confirm();
    if (big) onbigpicture();
  }

  // Controller and keyboard in big picture: up/down/left/right move
  // between this step's buttons, ✕ presses, ○ goes back a step. (In
  // desktop mode the controller already moves the focus around the window.)
  let root: HTMLDivElement | undefined = $state();
  const buttons = () => [...(root?.querySelectorAll<HTMLButtonElement>(".step button:not(:disabled)") ?? [])];
  $effect(() => {
    step;
    queueMicrotask(() => buttons()[buttons().length - 1]?.focus());
  });
  $effect(() =>
    useInput((intent) => {
      const bs = buttons();
      const at = bs.indexOf(document.activeElement as HTMLButtonElement);
      switch (intent) {
        case "left":
        case "up":
          if (at > 0) (bs[at - 1].focus(), feedback.move());
          else feedback.edge();
          return;
        case "right":
        case "down":
          if (at < bs.length - 1) (bs[at + 1].focus(), feedback.move());
          else feedback.edge();
          return;
        case "confirm":
          (bs[at] ?? bs[bs.length - 1])?.click();
          return;
        case "back":
          if (step > 0) ((step -= 1), feedback.move());
          else feedback.edge();
          return;
      }
    }),
  );

  const layouts = [
    { id: "deck", label: "Deck", about: "Rows of games, like a console's home screen" },
    { id: "console", label: "Console", about: "One big game at a time, with its details" },
    { id: "orbit", label: "Orbit", about: "Every game as a bubble you glide across" },
  ] as const;
</script>

<div class="welcome" class:big={mode === "bigpicture"} bind:this={root} role="dialog" aria-label="Welcome to Seaglass">
  <div class="card">
    <ol class="dots" aria-label="Steps">
      {#each steps as name, k (name)}<li class:on={k === step} class:done={k < step}><span></span>{name}</li>{/each}
    </ol>

    {#if step === 0}
      <div class="step">
        <h1>Welcome to Seaglass</h1>
        <p>It finds the games on this PC by itself: Steam, Epic, GOG, Xbox, EA, Ubisoft and Battle.net installs, other installers and games in plain folders, and gives them art and details.</p>
        <div class="count">
          <Icon name="scan" size={28} />
          {#if lib.scan.running && !games}Looking for games…{:else}Found {games} {games === 1 ? "game" : "games"}{lib.scan.running ? " so far…" : ""}{/if}
        </div>
        <div class="row">
          <button type="button" class="ghost" onclick={() => finish(false)}>Skip</button>
          <button type="button" class="primary" onclick={() => (step = 1)}>Next</button>
        </div>
      </div>
    {:else if step === 1}
      <div class="step">
        <h1>Where your games are</h1>
        <p>Store libraries are found on their own. Folders full of games are watched too, so a new one shows up by itself:</p>
        <ul class="folders">
          {#each [...autoFolders, ...(s?.folders ?? [])] as f (f)}<li><Icon name="folder" size={22} />{f}</li>{/each}
          {#if autoFolders.length + (s?.folders.length ?? 0) === 0}<li class="muted">No game folders found yet.</li>{/if}
        </ul>
        <div class="row">
          <button type="button" class="ghost" onclick={() => (step = 0)}>Back</button>
          <button type="button" onclick={addFolder}><Icon name="plus" size={20} />Add a folder</button>
          <button type="button" class="primary" onclick={() => (step = 2)}>Next</button>
        </div>
      </div>
    {:else if step === 2}
      <div class="step">
        <h1>How you play</h1>
        <p>Big picture is made for a controller and a TV; desktop mode for a mouse. Switch any time with F11 or the PS / Xbox button.</p>
        <div class="choices">
          {#each layouts as l (l.id)}
            <button type="button" class="choice" class:chosen={s?.bigPictureLayout === l.id} onclick={() => save({ bigPictureLayout: l.id })}>
              <span class="cl">{l.id === "deck" ? `${l.label} · default` : l.label}</span>
              <span class="ca">{l.about}</span>
            </button>
          {/each}
        </div>
        <div class="toggles">
          <button type="button" class="toggle" class:yes={s?.openBigPictureOnController} onclick={() => save({ openBigPictureOnController: !s?.openBigPictureOnController })}>
            <span class="knob"></span>Open big picture when a controller connects
          </button>
          <button type="button" class="toggle" class:yes={s?.startInBigPicture} onclick={() => save({ startInBigPicture: !s?.startInBigPicture })}>
            <span class="knob"></span>Start in big picture
          </button>
        </div>
        <div class="row">
          <button type="button" class="ghost" onclick={() => (step = 1)}>Back</button>
          <button type="button" class="primary" onclick={() => (step = 3)}>Next</button>
        </div>
      </div>
    {:else}
      <div class="step">
        <h1>Ready</h1>
        <p>
          {games ? `${games} ${games === 1 ? "game is" : "games are"} ready.` : "Games show up as they're found."}
          Art and details keep arriving in the background. Games found only by their folder name wait in <b>New on this PC</b> for a quick check.
        </p>
        {#if pad.connected}<p class="muted">{pad.name} is connected.</p>{/if}
        <div class="row">
          <button type="button" class="ghost" onclick={() => (step = 2)}>Back</button>
          {#if mode === "desktop"}<button type="button" onclick={() => finish(true)}><Icon name="tv" size={20} />Big picture</button>{/if}
          <button type="button" class="primary" onclick={() => finish(false)}>Start playing</button>
        </div>
      </div>
    {/if}
    {#if input.source === "pad" || mode === "bigpicture"}<p class="keys">✕ / A to choose · ○ / B to go back</p>{/if}
  </div>
</div>

<style>
  .welcome {
    position: fixed;
    inset: 0;
    z-index: 90;
    display: flex;
    align-items: center;
    justify-content: center;
    background: color-mix(in oklab, var(--bg, #0a0e13) 70%, transparent);
    backdrop-filter: blur(6px);
    padding: 24px;
  }
  .card {
    width: min(760px, 100%);
    max-height: 100%;
    overflow: auto;
    padding: 36px 40px 28px;
    border-radius: 22px;
    background: var(--surface, #121820);
    color: var(--text, #e8edf2);
    border: 1px solid var(--line, rgba(255, 255, 255, 0.1));
    box-shadow: 0 30px 80px rgba(0, 0, 0, 0.35);
  }
  .big .card {
    width: min(1100px, 100%);
    font-size: 1.3em;
  }
  .dots {
    display: flex;
    gap: 22px;
    margin: 0 0 26px;
    padding: 0;
    list-style: none;
    font-size: 13px;
    font-weight: 600;
    color: var(--muted, rgba(232, 237, 242, 0.55));
  }
  .dots li {
    display: flex;
    align-items: center;
    gap: 8px;
  }
  .dots span {
    width: 9px;
    height: 9px;
    border-radius: 50%;
    background: currentColor;
    opacity: 0.4;
  }
  .dots .on {
    color: var(--accent, oklch(0.75 0.13 205));
  }
  .dots .on span,
  .dots .done span {
    opacity: 1;
  }
  h1 {
    margin: 0 0 12px;
    font-family: var(--font-display);
    font-size: 2.1em;
  }
  p {
    margin: 0 0 18px;
    line-height: 1.5;
  }
  .muted {
    color: var(--muted, rgba(232, 237, 242, 0.6));
  }
  .count {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 16px 18px;
    border-radius: 14px;
    background: color-mix(in oklab, var(--text, #e8edf2) 6%, transparent);
    font-weight: 700;
    font-size: 1.15em;
    margin-bottom: 22px;
  }
  .folders {
    list-style: none;
    margin: 0 0 22px;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .folders li {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 10px 14px;
    border-radius: 10px;
    background: color-mix(in oklab, var(--text, #e8edf2) 6%, transparent);
    word-break: break-all;
  }
  .choices {
    display: grid;
    grid-template-columns: repeat(3, 1fr);
    gap: 12px;
    margin-bottom: 16px;
  }
  .choice {
    display: flex;
    flex-direction: column;
    align-items: flex-start;
    gap: 6px;
    padding: 16px;
    text-align: left;
    border-radius: 14px;
    border: 2px solid var(--line, rgba(255, 255, 255, 0.12));
    background: color-mix(in oklab, var(--text, #e8edf2) 6%, transparent);
    color: inherit;
    font: inherit;
  }
  .choice.chosen {
    border-color: var(--accent, oklch(0.75 0.13 205));
  }
  .cl {
    font-weight: 700;
  }
  .ca {
    font-size: 0.88em;
    color: var(--muted, rgba(232, 237, 242, 0.65));
  }
  .toggles {
    display: flex;
    flex-direction: column;
    gap: 8px;
    margin-bottom: 22px;
  }
  .toggle {
    display: flex;
    align-items: center;
    gap: 12px;
    padding: 8px 4px;
    border: 0;
    background: none;
    color: inherit;
    font: inherit;
    text-align: left;
  }
  .knob {
    width: 40px;
    height: 22px;
    border-radius: 999px;
    background: var(--line, rgba(255, 255, 255, 0.2));
    position: relative;
    flex-shrink: 0;
  }
  .knob::after {
    content: "";
    position: absolute;
    left: 3px;
    top: 3px;
    width: 16px;
    height: 16px;
    border-radius: 50%;
    background: #fff;
    transition: transform 0.15s;
  }
  .toggle.yes .knob {
    background: var(--accent, oklch(0.75 0.13 205));
  }
  .toggle.yes .knob::after {
    transform: translateX(18px);
  }
  .row {
    display: flex;
    justify-content: flex-end;
    gap: 10px;
    flex-wrap: wrap;
  }
  .row button {
    display: inline-flex;
    align-items: center;
    gap: 8px;
    padding: 10px 18px;
    border-radius: 10px;
    border: 1px solid var(--line, rgba(255, 255, 255, 0.14));
    background: color-mix(in oklab, var(--text, #e8edf2) 6%, transparent);
    color: inherit;
    font: inherit;
    font-weight: 600;
  }
  .row .primary {
    background: var(--accent, oklch(0.75 0.13 205));
    border-color: transparent;
    color: var(--accent-ink, #fff);
  }
  .row .ghost {
    background: none;
    border-color: transparent;
    margin-right: auto;
  }
  button:focus-visible,
  .big button:focus {
    outline: 3px solid var(--accent, oklch(0.75 0.13 205));
    outline-offset: 2px;
  }
  .keys {
    margin: 16px 0 0;
    font-size: 0.85em;
    color: var(--muted, rgba(232, 237, 242, 0.5));
    text-align: right;
  }
</style>
