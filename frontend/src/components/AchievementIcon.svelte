<script lang="ts">
  // An achievement's icon: its own picture (the locked one when it's
  // locked, or the unlocked one greyed out when there's no locked one), and
  // a trophy when there's no picture or it doesn't load.
  import Icon from "./Icon.svelte";
  import type { Achievement } from "../lib/types";

  let { a, size = 48, masked = false }: { a: Achievement; size?: number; masked?: boolean } = $props();

  let failed = $state(false);
  const src = $derived(masked ? "" : a.unlocked ? a.icon : a.iconGray || a.icon);
  const gray = $derived(!a.unlocked && !a.iconGray);
  $effect(() => {
    void src;
    failed = false;
  });
</script>

<span class="ach-icon" class:locked={!a.unlocked} style="--s: {size}px">
  {#if src && !failed}
    <img {src} alt="" class:gray loading="lazy" decoding="async" draggable="false" onerror={() => (failed = true)} />
  {:else}
    <span class="fallback"><Icon name={masked ? "lock" : "trophy"} size={Math.round(size * 0.55)} /></span>
  {/if}
</span>

<style>
  .ach-icon {
    width: var(--s);
    height: var(--s);
    flex: none;
    border-radius: calc(var(--s) * 0.16);
    overflow: hidden;
    display: inline-grid;
    place-items: center;
    background: color-mix(in srgb, currentColor 8%, transparent);
  }
  img {
    width: 100%;
    height: 100%;
    object-fit: cover;
    display: block;
  }
  img.gray {
    filter: grayscale(1) brightness(0.6);
  }
  .fallback {
    display: grid;
    place-items: center;
    opacity: 0.75;
  }
  .locked .fallback {
    opacity: 0.4;
  }
</style>
