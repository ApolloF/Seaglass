<script lang="ts">
  // Settings → Add-ons: what's installed, what each one may do, and the
  // explicit approval that turns one on.
  import Icon from "../components/Icon.svelte";
  import { api } from "../lib/api";
  import { lib } from "../lib/store.svelte";
  import type { AddonView } from "../lib/types";

  let list = $state<AddonView[]>([]);
  let loaded = $state(false);
  let reviewing = $state<string | null>(null);

  async function load() {
    const l = await lib.run(() => api.addons.list());
    if (l) list = l;
    loaded = true;
  }
  $effect(() => {
    load();
  });

  const replace = (v: AddonView) => (list = list.map((a) => (a.id === v.id ? v : a)));

  async function approve(a: AddonView) {
    const v = await lib.run(() => api.addons.enable(a.id, a.sha256));
    if (v) {
      replace(v);
      reviewing = null;
      lib.toast(`${a.name} is on`);
    } else load();
  }
  async function turnOff(a: AddonView) {
    const v = await lib.run(() => api.addons.disable(a.id));
    if (v) replace(v);
  }
  async function add() {
    const v = await lib.run(() => api.addons.add());
    if (v) {
      await load();
      reviewing = v.id;
    }
  }
  async function remove(a: AddonView) {
    await lib.run(() => api.addons.remove(a.id));
    load();
  }

  const stateText: Record<AddonView["state"], string> = {
    on: "On",
    off: "Off",
    changed: "Changed since you approved it",
    missing: "Program not found",
    broken: "Can't be used",
  };
</script>

<p class="lead">
  Add-ons are separate programs that add information and actions to your games, and can run a step before a game starts. They're off until you turn them on,
  and they have to be approved again when their program changes.
</p>

<div class="row">
  <button type="button" class="btn primary" onclick={add}><Icon name="plus" size={16} />Add add-on…</button>
  <button type="button" class="btn" onclick={() => lib.run(() => api.addons.openFolder())}><Icon name="folder" size={16} />Open add-ons folder</button>
</div>

{#if loaded && list.length === 0}
  <p class="empty">No add-ons yet. DLSS Updater adds itself: in DLSS Updater, choose <em>Settings → General → Connect to Seaglass</em>. For other add-ons, pick their <code>addon.json</code> with Add add-on.</p>
{/if}

<ul class="addons">
  {#each list as a (a.id)}
    <li class="addon" class:on={a.state === "on"} class:warn={a.state === "changed" || a.state === "missing" || a.state === "broken"}>
      <div class="head">
        <div class="title">
          <strong>{a.name}</strong>
          <span class="meta">{[a.version && `v${a.version}`, a.publisher && `by ${a.publisher}`].filter(Boolean).join(" · ")}</span>
        </div>
        <span class="state">{stateText[a.state]}</span>
      </div>
      {#if a.description}<p>{a.description}</p>{/if}
      {#if a.error}<p class="err">{a.error}</p>{/if}

      {#if a.state !== "broken"}
        {#if reviewing === a.id || a.state === "changed"}
          <div class="review">
            <strong>{a.state === "changed" ? "The add-on's program changed. Approve it again?" : `Turn on ${a.name}?`}</strong>
            <dl>
              <dt>It may</dt>
              <dd>
                {#if a.permissions.length}
                  {a.permissions.map((p) => p.label).join(", ")}
                {:else}
                  Only show information
                {/if}
              </dd>
              <dt>Program</dt>
              <dd class="path">{a.exe}</dd>
              <dt>Signature</dt>
              <dd>{a.signed ? "Signed (Authenticode)" : "Not signed. Only turn it on if you trust where it came from."}</dd>
              <dt>SHA-256</dt>
              <dd class="path">{a.sha256}</dd>
            </dl>
            <p class="small">It runs with your normal user rights, never as administrator and never inside games.</p>
            <div class="row">
              <button type="button" class="btn primary" onclick={() => approve(a)}>{a.state === "changed" ? "Approve" : "Turn on"}</button>
              <button type="button" class="btn" onclick={() => (a.state === "changed" ? turnOff(a) : (reviewing = null))}>{a.state === "changed" ? "Turn off" : "Cancel"}</button>
            </div>
          </div>
        {:else}
          <div class="row">
            {#if a.state === "on"}
              <button type="button" class="btn" onclick={() => turnOff(a)}>Turn off</button>
            {:else if a.state === "off"}
              <button type="button" class="btn" onclick={() => (reviewing = a.id)}>Turn on…</button>
            {/if}
            <button type="button" class="btn quiet" onclick={() => remove(a)}>Remove</button>
          </div>
        {/if}
      {:else}
        <div class="row"><button type="button" class="btn quiet" onclick={() => remove(a)}>Remove</button></div>
      {/if}
    </li>
  {/each}
</ul>

<style>
  .lead,
  .empty {
    margin: 0;
    color: var(--muted);
    font-size: 14px;
    line-height: 1.5;
  }
  .row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  .btn {
    height: 34px;
    padding: 0 12px;
    border-radius: var(--radius-s);
    border: 1px solid var(--line-strong);
    background: transparent;
    color: var(--text);
    font-size: 13.5px;
    font-weight: 700;
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }
  .btn:hover {
    background: var(--surface-3);
  }
  .btn.primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-ink);
  }
  .btn.primary:hover {
    filter: brightness(1.08);
  }
  .btn.quiet {
    border-color: transparent;
    color: var(--muted);
  }
  .addons {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .addon {
    padding: 14px 16px;
    border-radius: var(--radius);
    background: var(--surface-2);
    border: 1px solid transparent;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }
  .addon.on {
    border-color: color-mix(in oklab, var(--accent) 40%, transparent);
  }
  .addon.warn {
    border-color: color-mix(in oklab, var(--warn) 45%, transparent);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 12px;
  }
  .title {
    flex: 1;
    min-width: 0;
    display: flex;
    align-items: baseline;
    gap: 10px;
    flex-wrap: wrap;
  }
  .meta,
  .state {
    font-size: 13px;
    color: var(--muted);
  }
  .on .state {
    color: var(--accent-text);
    font-weight: 700;
  }
  .warn .state {
    color: var(--warn);
    font-weight: 700;
  }
  .addon p {
    margin: 0;
    font-size: 13.5px;
    line-height: 1.45;
    color: var(--text-2);
  }
  .addon p.err {
    color: var(--danger);
  }
  .review {
    display: flex;
    flex-direction: column;
    gap: 10px;
    padding: 12px 14px;
    border-radius: var(--radius-s);
    background: var(--surface);
    border: 1px solid var(--line-strong);
  }
  .review dl {
    margin: 0;
    display: grid;
    grid-template-columns: 90px 1fr;
    gap: 6px 12px;
    font-size: 13.5px;
  }
  .review dt {
    color: var(--muted);
  }
  .review dd {
    margin: 0;
    color: var(--text-2);
  }
  .path {
    word-break: break-all;
    user-select: text;
    font-size: 12.5px;
  }
  .small {
    font-size: 12.5px !important;
    color: var(--muted) !important;
  }
  code {
    font-size: 12.5px;
  }
</style>
