<script lang="ts">
  // Where updating stands, with the one action that makes sense next.
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { ago } from "../lib/format";
  import { lib } from "../lib/store.svelte";

  const u = $derived(lib.update);
  const pct = $derived(Math.round((u?.progress ?? 0) * 100));
</script>

{#if u}
  <div class="status" aria-live="polite">
    {#if u.status === "off"}
      <p class="line">This is a development build, so it doesn't update itself.</p>
    {:else if u.status === "checking"}
      <p class="line"><span class="spin"><Icon name="refresh" size={16} stroke={2} /></span>Checking for updates…</p>
    {:else if u.status === "downloading"}
      <p class="line"><Icon name="download" size={16} stroke={2} />Downloading {u.latest} · {pct}%</p>
      <div class="bar" role="progressbar" aria-valuemin={0} aria-valuemax={100} aria-valuenow={pct}><i style:width="{pct}%"></i></div>
    {:else if u.status === "ready"}
      <p class="line strong"><Icon name="sparkle" size={16} stroke={2} />Seaglass {u.latest} is ready to install.</p>
      {#if u.failed}
        <p class="note">It didn't install when Seaglass last started. Try again, or download it from GitHub.</p>
      {:else}
        <p class="note">It installs the next time Seaglass starts (or while it waits in the tray), or now with a quick restart.</p>
      {/if}
      <div class="actions">
        <button type="button" class="btn primary" onclick={() => lib.installUpdate()}><Icon name="refresh" size={16} stroke={2} />Restart and update</button>
        <button type="button" class="btn" onclick={() => lib.run(() => api.updates.openReleasePage())}><Icon name="link" size={16} />What's new</button>
      </div>
    {:else if u.status === "available"}
      <p class="line strong"><Icon name="sparkle" size={16} stroke={2} />Seaglass {u.latest} is out.</p>
      <p class="note">This copy can't update itself (its folder isn't writable). Download the new version from GitHub.</p>
      <div class="actions">
        <button type="button" class="btn primary" onclick={() => lib.run(() => api.updates.openReleasePage())}><Icon name="download" size={16} />Download</button>
      </div>
    {:else}
      {#if u.status === "error"}
        <p class="line err"><Icon name="warn" size={16} stroke={2} />{u.error || "Checking for updates failed."}</p>
      {:else if u.status === "uptodate"}
        <p class="line"><Icon name="check" size={16} stroke={2.4} />You have the newest version.<span class="muted">Checked {ago(u.checkedAt).toLowerCase()}.</span></p>
      {/if}
      <div class="actions">
        <button type="button" class="btn" onclick={() => api.updates.check()}><Icon name="refresh" size={16} stroke={2} />Check for updates</button>
      </div>
    {/if}
  </div>
{/if}

<style>
  .status {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .line {
    margin: 0;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14.5px;
    color: var(--text-2);
  }
  .line.strong {
    color: var(--text);
    font-weight: 700;
  }
  .line.err {
    color: var(--danger);
  }
  .muted {
    color: var(--muted);
  }
  .note {
    margin: 0;
    font-size: 13.5px;
    color: var(--muted);
  }
  .bar {
    height: 6px;
    border-radius: 99px;
    background: var(--surface-3);
    overflow: hidden;
  }
  .bar i {
    display: block;
    height: 100%;
    background: var(--accent);
    transition: width 0.3s var(--ease);
  }
  .actions {
    display: flex;
    gap: 10px;
    flex-wrap: wrap;
  }
  .btn {
    width: fit-content;
    height: 38px;
    padding: 0 14px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: transparent;
    display: flex;
    align-items: center;
    gap: 8px;
    font-size: 14.5px;
    font-weight: 700;
  }
  .btn:hover {
    background: var(--surface-2);
  }
  .btn.primary {
    border-color: transparent;
    background: var(--accent);
    color: var(--accent-ink);
  }
  .btn.primary:hover {
    filter: brightness(1.08);
  }
  .spin {
    display: inline-flex;
    animation: spin 1s linear infinite;
  }
  @keyframes spin {
    to {
      transform: rotate(360deg);
    }
  }
  @media (prefers-reduced-motion: reduce) {
    .spin {
      animation: none;
    }
  }
</style>
