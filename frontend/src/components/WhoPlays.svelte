<script lang="ts">
  // Who's playing on this PC, with Syncer's accounts to switch between:
  // their saves, playtime, achievements and settings come along.
  import { accountColor, hasAccounts, initial, playing } from "../lib/profile";
  import { lib } from "../lib/store.svelte";

  let open = $state(false);
  let box = $state<HTMLElement>();
  let busy = $state("");
  const p = $derived(lib.profile);
  const me = $derived(playing(p));

  async function pick(id: string) {
    if (!p || busy || id === me?.id) {
      open = false;
      return;
    }
    busy = id;
    await lib.switchAccount(id);
    busy = "";
    open = false;
  }
</script>

<svelte:window
  onpointerdown={(e) => open && box && !box.contains(e.target as Node) && (open = false)}
  onkeydown={(e) => open && e.key === "Escape" && (open = false)}
/>

{#if hasAccounts(p)}
  <div class="who" bind:this={box}>
    {#if open}
      <div class="menu" role="menu" aria-label="Who's playing">
        {#each p.accounts as a (a.id)}
          <button type="button" role="menuitemradio" aria-checked={a.id === me?.id} class="acc" class:on={a.id === me?.id} disabled={!!busy} onclick={() => pick(a.id)}>
            <span class="av" style:background={accountColor(a)}>{initial(a.name)}</span>
            <span class="grow">{a.name}</span>
            {#if busy === a.id}<span class="muted">Switching…</span>{:else if a.id === me?.id}<span class="muted">Playing</span>{/if}
          </button>
        {/each}
        <p class="hint">Switching puts their saves in place and shows their playtime and achievements.</p>
      </div>
    {/if}
    <button type="button" class="me" aria-expanded={open} onclick={() => (open = !open)} title="Who's playing on this PC">
      <span class="av" style:background={accountColor(me)}>{initial(me?.name ?? p.ownerName ?? "?")}</span>
      <span class="grow">{me ? `${me.name} is playing` : "Who's playing?"}</span>
    </button>
  </div>
{/if}

<style>
  .who {
    position: relative;
  }
  .me,
  .acc {
    width: 100%;
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 7px 10px;
    border: 0;
    border-radius: 10px;
    background: transparent;
    color: var(--text-2);
    font-size: 13.5px;
    font-weight: 600;
    text-align: left;
  }
  .me:hover,
  .acc:hover:not(:disabled) {
    background: var(--surface-2);
  }
  .acc.on {
    color: var(--accent-text);
  }
  .av {
    width: 24px;
    height: 24px;
    flex-shrink: 0;
    border-radius: 50%;
    display: grid;
    place-items: center;
    background: var(--accent);
    color: #fff;
    font-size: 12px;
    font-weight: 800;
  }
  .grow {
    flex: 1;
    white-space: nowrap;
    overflow: hidden;
    text-overflow: ellipsis;
  }
  .muted {
    color: var(--muted);
    font-size: 12px;
  }
  .menu {
    position: absolute;
    left: 0;
    right: 0;
    bottom: calc(100% + 6px);
    z-index: 20;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 6px;
    border-radius: 12px;
    border: 1px solid var(--line);
    background: var(--surface);
    box-shadow: 0 12px 32px rgba(0, 0, 0, 0.3);
  }
  .hint {
    margin: 4px 8px 2px;
    color: var(--muted);
    font-size: 12px;
    line-height: 1.4;
  }
</style>
