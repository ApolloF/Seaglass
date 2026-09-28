<script lang="ts">
  // The panel the Options / PS button opens: controller, quick toggles and
  // the ways out of big picture.
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { feedback, input, pad, useInput } from "../lib/input.svelte";
  import { hasAccounts, playing } from "../lib/profile";
  import { lib } from "../lib/store.svelte";
  import type { Settings } from "../lib/types";
  import Hints from "./Hints.svelte";

  let { light, onclose, onsettings, ondesktop }: { light: string; onclose: () => void; onsettings: () => void; ondesktop: () => void } = $props();

  type Item = { id: string; title: string; detail: string; toggle?: keyof Settings; run?: () => void; current?: boolean };
  // Choosing who's playing lists the accounts in place of the items.
  let choosing = $state(false);
  let switching = $state("");
  const accountItems = $derived.by((): Item[] => {
    const p = lib.profile;
    if (!hasAccounts(p)) return [];
    const me = playing(p);
    return [
      ...p.accounts.map((a) => ({
        id: "acc:" + a.id,
        title: a.name,
        detail: switching === a.id ? "Putting their saves in place…" : a.id === me?.id ? "Playing on this PC" : "Their saves, playtime and achievements",
        current: a.id === me?.id,
        run: () => void choose(a.id),
      })),
      { id: "back", title: "Back", detail: "", run: () => ((choosing = false), (i = 0)) },
    ];
  });
  async function choose(id: string) {
    if (switching) return;
    if (id !== playing(lib.profile)?.id) {
      switching = id;
      await lib.switchAccount(id);
      switching = "";
    }
    choosing = false;
    i = 0;
  }
  const items = $derived.by((): Item[] => (choosing && accountItems.length ? accountItems : mainItems));
  const mainItems = $derived.by((): Item[] => [
    ...(hasAccounts(lib.profile)
      ? [{ id: "who", title: `${playing(lib.profile)?.name ?? "Nobody"} is playing`, detail: "Switch to someone else's saves", run: () => ((choosing = true), (i = 0)) }]
      : []),
    {
      id: "scan",
      title: "Look for games",
      detail: lib.scan.running ? "Looking…" : `${lib.counts.all} games${lib.scan.tookMs ? ` · last scan took ${(lib.scan.tookMs / 1000).toFixed(1)} s` : ""}`,
      run: () => api.rescan(),
    },
    { id: "haptics", title: "Haptics", detail: "A tick as you move, a bump at the end", toggle: "haptics" },
    { id: "lightbar", title: "Lightbar follows the game", detail: "Tints the DualSense to the selected game", toggle: "lightbar" },
    { id: "sounds", title: "Navigation sounds", detail: "Soft clicks as you move", toggle: "sounds" },
    { id: "settings", title: "Settings", detail: "Layout, controller and more", run: onsettings },
    { id: "desktop", title: "Switch to desktop mode", detail: input.source === "keyboard" ? "Mouse and keyboard layout · F11 from anywhere" : "Mouse and keyboard layout", run: ondesktop },
  ]);

  let i = $state(0);

  function activate(it: Item) {
    feedback.confirm();
    if (it.toggle && lib.settings) {
      const k = it.toggle;
      lib.saveSettings({ ...lib.settings, [k]: !lib.settings[k] });
    } else it.run?.();
  }

  $effect(() =>
    useInput((intent) => {
      switch (intent) {
        case "up":
        case "down": {
          const j = i + (intent === "up" ? -1 : 1);
          if (j >= 0 && j < items.length) ((i = j), feedback.move());
          else feedback.edge();
          return;
        }
        case "confirm":
          activate(items[i]);
          return;
        case "back":
          if (choosing) {
            choosing = false;
            i = 0;
            return;
          }
          onclose();
          return;
        case "menu":
        case "home":
        case "left":
          onclose();
          return;
      }
    }),
  );

  const now = new Date();
  const clock = `${String(now.getHours()).padStart(2, "0")}:${String(now.getMinutes()).padStart(2, "0")}`;
</script>

<div class="scrim" role="presentation" onclick={onclose}></div>
<aside class="qa" aria-label="Quick access">
  <div class="top"><span class="h">Quick access</span><span class="clock">{clock}</span></div>
  <div class="pad">
    <div class="pad-row">
      <Icon name="pad" size={26} stroke={1.8} />
      <span class="grow">{pad.connected ? pad.name || "Controller" : "No controller"}</span>
      {#if pad.connected}<span class="muted">{pad.wireless ? "Bluetooth" : "USB"}{pad.battery >= 0 ? ` · ${pad.battery}%` : ""}</span>{/if}
    </div>
    {#if pad.connected && pad.battery >= 0}
      <div class="battery"><span style:width="{pad.battery}%"></span></div>
    {/if}
    {#if pad.dualSense}
      <div class="pad-row small"><span class="muted">Lightbar</span><span class="swatch" style:background={lib.settings?.lightbar ? light : "#e8edf2"}></span></div>
    {/if}
  </div>
  {#each items as it, k (it.id)}
    <button type="button" class="item" class:on={k === i} onclick={() => ((i = k), activate(it))} aria-pressed={it.toggle ? !!lib.settings?.[it.toggle] : undefined}>
      <span class="text"><span class="t">{it.title}</span><span class="d">{it.detail}</span></span>
      {#if it.current}
        <span class="cur">Playing</span>
      {:else if it.toggle}
        <span class="track" class:yes={!!lib.settings?.[it.toggle]}><span class="knob"></span></span>
      {:else}
        <Icon name="chevronDown" size={20} stroke={2.2} />
      {/if}
    </button>
  {/each}
  <div class="grow"></div>
  <Hints hints={[{ button: "confirm", label: "Select" }, { button: "back", label: "Close" }]} />
</aside>

<style>
  .cur {
    font-size: 15px;
    font-weight: 700;
    color: var(--accent-text, #9fd3ff);
  }
  .scrim {
    position: absolute;
    inset: 0;
    z-index: 30;
    background: rgba(4, 6, 9, 0.45);
  }
  .qa {
    position: absolute;
    right: 0;
    top: 0;
    bottom: 0;
    width: 520px;
    z-index: 31;
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 40px 34px 40px;
    background: rgba(13, 19, 25, 0.97);
    border-left: 1px solid rgba(255, 255, 255, 0.08);
    color: #e8edf2;
    animation: slide 0.32s cubic-bezier(0.2, 0.8, 0.2, 1) both;
  }
  @keyframes slide {
    from {
      transform: translateX(100%);
    }
  }
  .top {
    display: flex;
    align-items: baseline;
    justify-content: space-between;
    margin-bottom: 6px;
  }
  .h {
    font-family: var(--font-display);
    font-size: 38px;
    font-weight: 700;
  }
  .clock {
    font-size: 24px;
    font-weight: 600;
    color: #9ba8b5;
  }
  .pad {
    display: flex;
    flex-direction: column;
    gap: 12px;
    padding: 18px 22px;
    border-radius: 18px;
    background: #121a23;
  }
  .pad-row {
    display: flex;
    align-items: center;
    gap: 12px;
    font-size: 21px;
    font-weight: 700;
  }
  .pad-row.small {
    font-size: 17px;
  }
  .grow {
    flex: 1;
  }
  .muted {
    font-size: 17px;
    font-weight: 600;
    color: #9ba8b5;
  }
  .battery {
    height: 8px;
    border-radius: 4px;
    background: rgba(255, 255, 255, 0.1);
  }
  .battery span {
    display: block;
    height: 100%;
    border-radius: 4px;
    background: oklch(0.8 0.12 205);
  }
  .swatch {
    flex: 1;
    height: 10px;
    border-radius: 5px;
  }
  .item {
    display: flex;
    align-items: center;
    gap: 16px;
    padding: 16px 20px;
    border: 0;
    border-radius: 16px;
    background: #121a23;
    color: #e8edf2;
    text-align: left;
  }
  .item.on {
    background: #1b2632;
    box-shadow: 0 0 0 3px #fff;
  }
  .item :global(svg) {
    transform: rotate(-90deg);
    color: #9ba8b5;
  }
  .text {
    flex: 1;
    display: flex;
    flex-direction: column;
    gap: 4px;
  }
  .t {
    font-size: 21px;
    font-weight: 700;
  }
  .d {
    font-size: 16px;
    color: #9ba8b5;
  }
  .track {
    position: relative;
    width: 56px;
    height: 32px;
    border-radius: 16px;
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
    width: 24px;
    height: 24px;
    border-radius: 50%;
    background: #fff;
    transition: transform 0.2s cubic-bezier(0.2, 0.8, 0.2, 1);
  }
  .track.yes .knob {
    transform: translateX(24px);
  }
</style>
