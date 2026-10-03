<script lang="ts">
  // Big picture settings, made for a controller: up/down to pick a row,
  // left/right or ✕ to change it. The list scrolls to keep the row in view.
  import { api } from "../lib/api";
  import { feedback, useInput } from "../lib/input.svelte";
  import { syncerSummary } from "../lib/saves";
  import { lib } from "../lib/store.svelte";
  import type { Settings, SyncerStatus } from "../lib/types";
  import Hints from "./Hints.svelte";

  let { onback, onpadtest }: { onback: () => void; onpadtest: () => void } = $props();

  type Row =
    | { key: keyof Settings; title: string; detail: string; kind: "toggle"; group?: string }
    | { key: keyof Settings; title: string; detail: string; kind: "choice"; options: [string | number, string][]; group?: string }
    | { key: "syncer"; title: string; detail: string; kind: "status"; group?: string }
    | { key: "padtest"; title: string; detail: string; kind: "action"; group?: string }
    | { key: `library:${string}`; source: string; title: string; detail: string; kind: "library"; group?: string };

  const fixed: Row[] = [
    {
      group: "Big picture",
      key: "bigPictureLayout",
      title: "Layout",
      detail: "How big picture looks. Deck is the default.",
      kind: "choice",
      options: [
        ["deck", "Deck"],
        ["console", "Console"],
        ["orbit", "Orbit"],
      ],
    },
    { key: "sounds", title: "Navigation sounds", detail: "Soft clicks as you move.", kind: "toggle" },
    { key: "startInBigPicture", title: "Start in big picture", detail: "Skip the desktop window when Seaglass starts.", kind: "toggle" },
    {
      group: "Controller",
      key: "glyphs",
      title: "Button prompts",
      detail: "Shapes for PlayStation controllers, letters for Xbox. Auto follows the controller in use; keys show while you use the keyboard.",
      kind: "choice",
      options: [
        ["auto", "Auto"],
        ["playstation", "PlayStation"],
        ["xbox", "Xbox"],
      ],
    },
    { key: "padtest", title: "Test controller", detail: "Every button, stick and trigger as the controller reports it, and each rumble.", kind: "action" },
    { key: "haptics", title: "Haptics", detail: "A tick as you move, a bump at the end of a row, a firmer pulse when you choose.", kind: "toggle" },
    { key: "lightbar", title: "Lightbar follows the game", detail: "Tints the DualSense to the selected game.", kind: "toggle" },
    { key: "openBigPictureOnController", title: "Open big picture when a controller connects", detail: "Switches over as soon as you pick one up.", kind: "toggle" },
    { key: "psButton", title: "PS / Xbox button opens Seaglass", detail: "From other apps, and the overlay in games. Turn off Steam's guide-button shortcut to avoid both opening.", kind: "toggle" },
    {
      key: "padWhilePlaying",
      title: "Controller while playing",
      detail: "Listen: the PS button opens the overlay, nothing is sent to the controller. Off: let go completely.",
      kind: "choice",
      options: [
        ["listen", "Listen"],
        ["off", "Off"],
      ],
    },
    { group: "Saves", key: "syncer", title: "Syncer", detail: "", kind: "status" },
    { key: "syncSavesBefore", title: "Sync saves before playing", detail: "Saves come up to date from your other PCs first.", kind: "toggle" },
    {
      key: "syncWait",
      title: "Wait for a sync at most",
      detail: "When another PC is slow to send a save, the game starts after this.",
      kind: "choice",
      options: [
        [30, "30 s"],
        [60, "1 min"],
        [150, "2½ min"],
        [300, "5 min"],
      ],
    },
    { key: "backupSavesAfter", title: "Back up saves after playing", detail: "A backup runs when the game exits.", kind: "toggle" },
    { key: "startSyncer", title: "Start Syncer when it isn't running", detail: "In the background, without its window.", kind: "toggle" },
    { key: "syncProfile", title: "Sync playtime, achievements and settings", detail: "Syncer takes them to your other PCs and backs them up.", kind: "toggle" },
    { key: "sameSettings", title: "Same settings on every PC", detail: "Settings changed on another PC are used here too. Game folders stay per PC.", kind: "toggle" },
    { key: "askWhoPlays", title: "Ask who's playing", detail: "For a PC several people take turns on. Off: games start as whoever is playing now.", kind: "toggle" },
    { group: "Library and playing", key: "showOwned", title: "Show games you own that aren't installed", detail: "From the store accounts connected in desktop Settings.", kind: "toggle" },
    { key: "closeWhilePlaying", title: "Close the interface while playing", detail: "Frees its memory. It comes back when the game exits.", kind: "toggle" },
    { key: "noticeExternal", title: "Notice games started elsewhere", detail: "Games started from Steam or a shortcut count their playtime here too.", kind: "toggle" },
    { key: "achievements", title: "Show achievements", detail: "From Steam, Epic, GOG and Steam emulators. After you play, Seaglass says what you unlocked.", kind: "toggle" },
    { key: "showHiddenAchievements", title: "Show hidden achievements", detail: "Their names before you unlock them. They can give away the story.", kind: "toggle" },
    { key: "autoUpdate", title: "Keep Seaglass up to date", detail: "New versions download in the background and install the next time Seaglass starts, or while it waits in the tray.", kind: "toggle" },
  ];

  // One row per library on this PC, to show or hide its games.
  const rows = $derived.by(() => {
    const libs: Row[] = lib.libraries.map((l, k) => ({
      group: k === 0 ? "Libraries" : undefined,
      key: `library:${l.id}` as const,
      source: l.id,
      title: `${l.label} games`,
      detail: `${l.count} ${l.count === 1 ? "game" : "games"}${l.hidden ? ", hidden" : ""}`,
      kind: "library" as const,
    }));
    const at = fixed.findIndex((r) => r.key === "showOwned");
    return [...fixed.slice(0, at), ...libs, ...fixed.slice(at)];
  });

  let i = $state(0);
  // The list can get shorter under the cursor (a library gone after a scan).
  $effect(() => {
    if (i >= rows.length) i = Math.max(0, rows.length - 1);
  });
  const s = $derived(lib.settings);

  let syncer = $state<SyncerStatus | null>(null);
  let checking = $state(false);
  async function check(start: boolean) {
    checking = true;
    try {
      syncer = await api.saves.syncer(start);
    } catch {
      /* keeps the last answer */
    } finally {
      checking = false;
    }
  }
  $effect(() => {
    check(false);
  });
  const syncerSum = $derived(syncerSummary(syncer, s?.startSyncer ?? true));
  let installing = $state(false);
  async function install() {
    if (installing) return;
    installing = true;
    await lib.run(() => api.saves.installSyncer());
    installing = false;
    check(false);
  }

  function change(dir: 1 | -1) {
    const row = rows[i];
    if (!s || !row) return;
    if (row.kind === "action") {
      feedback.confirm();
      onpadtest();
      return;
    }
    if (row.kind === "status") {
      // ✕ on Syncer: start it (or look again), or open it to sort things out.
      if (syncerSum?.action === "open") lib.run(() => api.saves.openSyncer());
      else if (syncerSum?.action === "get" || syncerSum?.action === "update") install();
      else check(true);
    } else if (row.kind === "library") {
      lib.toggleLibrary(row.source);
    } else if (row.kind === "toggle") {
      lib.saveSettings({ ...s, [row.key]: !s[row.key] });
    } else {
      const ids = row.options.map((o) => o[0]);
      const cur = ids.findIndex((v) => String(v) === String(s[row.key]));
      const next = ids[(cur + dir + ids.length) % ids.length];
      lib.saveSettings({ ...s, [row.key]: next } as Settings);
    }
    feedback.confirm();
  }

  let list: HTMLDivElement | undefined = $state();
  $effect(() => {
    list?.querySelector<HTMLElement>(`[data-row="${i}"]`)?.scrollIntoView({ block: "nearest", behavior: "smooth" });
  });

  $effect(() =>
    useInput((intent) => {
      if (intent === "up" || intent === "down") {
        const j = i + (intent === "up" ? -1 : 1);
        if (j >= 0 && j < rows.length) ((i = j), feedback.move());
        else feedback.edge();
      } else if (intent === "confirm" || (intent === "right" && rows[i]?.kind !== "action")) change(1);
      else if (intent === "left") change(-1);
      else if (intent === "back") onback();
      else return false;
    }),
  );

  function valueLabel(row: Row): string {
    if (!s || row.kind === "status" || row.kind === "action" || row.kind === "library") return "";
    if (row.kind === "toggle") return s[row.key] ? "On" : "Off";
    return row.options.find((o) => String(o[0]) === String(s[row.key]))?.[1] ?? "";
  }
</script>

<div class="settings">
  <h1>Settings</h1>
  <div class="rows" bind:this={list}>
    {#each rows as row, k (row.key)}
      {#if row.group}<span class="group">{row.group}</span>{/if}
      <button type="button" class="row" class:on={k === i} data-row={k} tabindex="-1" onclick={() => ((i = k), change(1))}>
        {#if row.kind === "status"}
          <span class="dot" class:ok={syncerSum?.tone === "ok"} class:warn={syncerSum?.tone === "warn"}></span>
          <span class="text"
            ><span class="t">Syncer: {checking && !syncerSum ? "Checking…" : (syncerSum?.title ?? "Checking…")}</span><span class="d">{syncerSum?.detail ?? ""}</span></span
          >
          {#if syncerSum}
            <span class="act"
              >{syncerSum.action === "open" ? "Open Syncer" : installing ? "Installing…" : syncerSum.action === "get" ? "Install Syncer" : syncerSum.action === "update" ? "Update" : syncerSum.action === "start" ? "Start" : "Check again"}</span
            >
          {/if}
        {:else}
          <span class="text"><span class="t">{row.title}</span><span class="d">{row.detail}</span></span>
          {#if row.kind === "action"}
            <span class="act">Open</span>
          {:else if row.kind === "library"}
            <span class="track" class:yes={!s?.hiddenSources.includes(row.source)}><span class="knob"></span></span>
          {:else if row.kind === "toggle"}
            <span class="track" class:yes={!!s?.[row.key]}><span class="knob"></span></span>
          {:else}
            <span class="choice"><span class="arrow">‹</span>{valueLabel(row)}<span class="arrow">›</span></span>
          {/if}
        {/if}
      </button>
    {/each}
    <p class="more">Library folders, store accounts and art sources are in desktop mode's settings.</p>
  </div>
  <div class="hints"><Hints hints={[{ button: "confirm", label: "Change" }, { button: "back", label: "Back" }]} /></div>
</div>

<style>
  .settings {
    position: absolute;
    inset: 0;
    background: #0a0e13;
    color: #e8edf2;
  }
  h1 {
    position: absolute;
    left: 110px;
    top: 50px;
    margin: 0;
    font-family: var(--font-display);
    font-size: 52px;
  }
  .rows {
    position: absolute;
    left: 110px;
    right: 110px;
    top: 140px;
    bottom: 120px;
    max-width: 1300px;
    overflow: hidden;
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 6px;
    mask-image: linear-gradient(180deg, transparent 0, #000 12px, #000 calc(100% - 40px), transparent 100%);
    /* The selected row scrolls fully clear of the faded edges. */
    scroll-padding-block: 18px 48px;
  }
  .group {
    margin: 18px 0 4px 6px;
    font-size: 17px;
    font-weight: 800;
    letter-spacing: 0.1em;
    text-transform: uppercase;
    color: #7f8c99;
  }
  .group:first-child {
    margin-top: 4px;
  }
  .row {
    flex-shrink: 0;
    display: flex;
    align-items: center;
    gap: 24px;
    padding: 20px 26px;
    border: 0;
    border-radius: 18px;
    background: #121a23;
    color: #e8edf2;
    text-align: left;
  }
  .row.on {
    background: #1b2632;
    box-shadow: 0 0 0 3px #fff;
  }
  .text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .t {
    font-size: 24px;
    font-weight: 700;
  }
  .d {
    font-size: 18px;
    color: #9ba8b5;
  }
  .dot {
    width: 16px;
    height: 16px;
    border-radius: 50%;
    flex-shrink: 0;
    background: #6b7885;
  }
  .dot.ok {
    background: oklch(0.75 0.15 150);
    box-shadow: 0 0 0 5px color-mix(in oklab, oklch(0.75 0.15 150) 25%, transparent);
  }
  .dot.warn {
    background: oklch(0.8 0.14 75);
    box-shadow: 0 0 0 5px color-mix(in oklab, oklch(0.8 0.14 75) 25%, transparent);
  }
  .act {
    font-size: 20px;
    font-weight: 700;
    color: oklch(0.88 0.09 205);
    white-space: nowrap;
  }
  .choice {
    display: flex;
    align-items: center;
    gap: 14px;
    font-size: 22px;
    font-weight: 700;
    min-width: 220px;
    justify-content: space-between;
  }
  .arrow {
    color: #9ba8b5;
    font-size: 30px;
  }
  .track {
    position: relative;
    width: 62px;
    height: 34px;
    border-radius: 17px;
    background: rgba(255, 255, 255, 0.18);
    flex-shrink: 0;
    transition: background 0.2s;
  }
  .track.yes {
    background: oklch(0.8 0.12 205);
  }
  .knob {
    position: absolute;
    top: 4px;
    left: 4px;
    width: 26px;
    height: 26px;
    border-radius: 50%;
    background: #fff;
    transition: transform 0.2s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .track.yes .knob {
    transform: translateX(28px);
  }
  .more {
    margin: 20px 0 30px 6px;
    font-size: 19px;
    color: #9ba8b5;
  }
  .hints {
    position: absolute;
    right: 96px;
    bottom: 40px;
    left: 110px;
  }
</style>
