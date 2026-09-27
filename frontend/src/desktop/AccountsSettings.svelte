<script lang="ts">
  // Settings → Library → Your store accounts: where owned games that
  // aren't installed come from. Everything here is optional and off by default.
  import Toggle from "../components/Toggle.svelte";
  import { api } from "../lib/api";
  import { ago } from "../lib/format";
  import { lib } from "../lib/store.svelte";
  import type { Accounts, StoreAccount } from "../lib/types";

  let acc = $state<Accounts | null>(null);
  let steamKey = $state("");
  let epicCode = $state("");
  let epicStarted = $state(false);
  let gogCode = $state("");
  let gogStarted = $state(false);
  let busy = $state<"" | "steam" | "epic" | "gog" | "gogSignIn">("");

  $effect(() => {
    api.accounts.get().then((a) => (acc = a));
    return api.accounts.onChange((a) => (acc = a));
  });

  const s = $derived(lib.settings);

  async function withBusy(which: typeof busy, fn: () => Promise<Accounts>) {
    busy = which;
    const a = await lib.run(fn);
    busy = "";
    if (a) acc = a;
    return !!a;
  }

  async function connectSteam() {
    if (await withBusy("steam", () => api.accounts.setSteamKey(steamKey.trim()))) {
      steamKey = "";
      lib.toast(`Steam connected: ${acc?.steam.games ?? 0} games`);
    }
  }
  async function connectEpic() {
    if (await withBusy("epic", () => api.accounts.epicSignIn(epicCode))) {
      epicCode = "";
      epicStarted = false;
      lib.toast(`Epic connected: ${acc?.epic.games ?? 0} games`);
    }
  }

  async function signInGOG() {
    if (await withBusy("gogSignIn", () => api.accounts.gogSignIn(gogCode))) {
      gogCode = "";
      gogStarted = false;
      lib.toast("Signed in to GOG: achievements show names, icons and progress");
    }
  }

  function status(a: StoreAccount): string {
    if (a.syncing) return "Updating…";
    if (a.error) return a.error;
    if (!a.connected) return "Not connected";
    return `${a.games} games${a.synced ? ` · updated ${ago(a.synced).toLowerCase()}` : ""}`;
  }
</script>

<div class="group">
  <span class="glabel">Your store accounts</span>
  {#if s}
    <Toggle
      checked={s.showOwned}
      title="Show games you own that aren't installed"
      detail="From the accounts connected below. They show with an Install button that opens the store to install them."
      onchange={(v) => lib.saveSettings({ ...s, showOwned: v })}
    />
  {/if}

  {#if acc}
    <div class="acct">
      <div class="head">
        <strong>Steam</strong>
        <span class="st" class:err={!!acc.steam.error}>{status(acc.steam)}</span>
        {#if acc.steam.connected}<button type="button" class="link" onclick={() => withBusy("steam", () => api.accounts.setSteamKey(""))}>Disconnect</button>{/if}
      </div>
      {#if !acc.steam.connected}
        <p>
          Uses your own Steam Web API key, stored encrypted for your Windows account. Your games must be visible in Steam's privacy settings (<em>Game details</em>).
          <button type="button" class="link" onclick={() => lib.run(() => api.accounts.openSteamKeyPage())}>Get a key</button>
        </p>
        <div class="row">
          <input type="password" bind:value={steamKey} placeholder="Steam Web API key" aria-label="Steam Web API key" autocomplete="off" spellcheck="false" />
          <button type="button" class="btn primary" disabled={!steamKey.trim() || busy === "steam"} onclick={connectSteam}>{busy === "steam" ? "Checking…" : "Connect"}</button>
        </div>
      {/if}
      <p class="note">The key also gives achievement names and icons to unofficial copies of Steam games that don't bring their own.</p>
    </div>

    <div class="acct">
      <div class="head">
        <strong>GOG</strong>
        <span class="st" class:err={!!acc.gog.error}>{acc.gog.available ? status(acc.gog) : "GOG Galaxy isn't installed"}</span>
      </div>
      <Toggle
        checked={acc.gog.connected}
        disabled={!acc.gog.available || busy === "gog"}
        title="Read GOG Galaxy's library"
        detail="Lists your GOG games from GOG Galaxy's own database on this PC. Nothing is sent anywhere."
        onchange={(v) => withBusy("gog", () => api.accounts.setGOG(v))}
      />
      <div class="head sub">
        <strong>Achievements</strong>
        <span class="st">{acc.gogSignIn?.connected ? "Signed in" : "Not signed in"}</span>
        {#if acc.gogSignIn?.connected}<button type="button" class="link" onclick={() => withBusy("gogSignIn", () => api.accounts.gogSignOut())}>Sign out</button>{/if}
      </div>
      {#if !acc.gogSignIn?.connected}
        {#if !gogStarted}
          <p>GOG Galaxy's database has your progress but not the achievements' names or icons. Sign in on gog.com in your browser to get those; Seaglass keeps only GOG's sign-in token, encrypted for your Windows account. GOG Galaxy stays signed in as it is.</p>
          <div class="row">
            <button type="button" class="btn" onclick={() => lib.run(() => api.accounts.openGOGSignIn()).then(() => (gogStarted = true))}>Sign in with GOG…</button>
          </div>
        {:else}
          <p>After signing in, the browser ends on a page whose address contains <code>code=</code>. Copy that whole address and paste it here.</p>
          <div class="row">
            <input type="password" bind:value={gogCode} placeholder="Address with code=…" aria-label="GOG sign-in address" autocomplete="off" spellcheck="false" />
            <button type="button" class="btn primary" disabled={!gogCode.trim() || busy === "gogSignIn"} onclick={signInGOG}>{busy === "gogSignIn" ? "Signing in…" : "Sign in"}</button>
            <button type="button" class="btn" onclick={() => (gogStarted = false)}>Cancel</button>
          </div>
        {/if}
      {/if}
    </div>

    <div class="acct">
      <div class="head">
        <strong>Epic</strong>
        <span class="st" class:err={!!acc.epic.error}>{acc.epic.connected && acc.epic.name ? `${acc.epic.name} · ` : ""}{status(acc.epic)}</span>
        {#if acc.epic.connected}<button type="button" class="link" onclick={() => withBusy("epic", () => api.accounts.epicSignOut())}>Sign out</button>{/if}
      </div>
      {#if !acc.epic.connected}
        {#if !epicStarted}
          <p>Sign in on epicgames.com in your browser. Seaglass never sees your password; it keeps only Epic's sign-in token, encrypted for your Windows account.</p>
          <div class="row">
            <button type="button" class="btn" onclick={() => lib.run(() => api.accounts.openEpicSignIn()).then(() => (epicStarted = true))}>Sign in with Epic…</button>
          </div>
        {:else}
          <p>After signing in, the page shows a short text with an <code>authorizationCode</code>. Copy that text (or just the code) and paste it here.</p>
          <div class="row">
            <input type="password" bind:value={epicCode} placeholder="authorizationCode" aria-label="Epic authorization code" autocomplete="off" spellcheck="false" />
            <button type="button" class="btn primary" disabled={!epicCode.trim() || busy === "epic"} onclick={connectEpic}>{busy === "epic" ? "Connecting…" : "Connect"}</button>
            <button type="button" class="btn" onclick={() => (epicStarted = false)}>Cancel</button>
          </div>
        {/if}
      {/if}
    </div>

    {#if acc.steam.connected || acc.gog.connected || acc.epic.connected}
      <div class="row"><button type="button" class="btn" onclick={() => api.accounts.sync()}>Update owned games now</button></div>
    {/if}
  {/if}
</div>

<style>
  .group {
    display: flex;
    flex-direction: column;
    gap: 8px;
  }
  .glabel {
    font-size: 14px;
    font-weight: 700;
    color: var(--text-2);
  }
  .acct {
    display: flex;
    flex-direction: column;
    gap: 8px;
    padding: 12px 14px;
    border-radius: var(--radius);
    background: var(--surface-2);
  }
  .head {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }
  .st {
    flex: 1;
    font-size: 13px;
    color: var(--muted);
  }
  .head.sub {
    margin-top: 4px;
  }
  .note {
    font-size: 12.5px;
    color: var(--muted);
  }
  .st.err {
    color: var(--warn);
  }
  p {
    margin: 0;
    font-size: 13.5px;
    line-height: 1.45;
    color: var(--text-2);
  }
  .row {
    display: flex;
    gap: 8px;
    flex-wrap: wrap;
  }
  input {
    flex: 1;
    min-width: 200px;
    height: 34px;
    padding: 0 10px;
    border-radius: var(--radius-s);
    border: 1px solid var(--line-strong);
    background: var(--surface);
    color: var(--text);
    font: inherit;
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
  }
  .btn:hover:not(:disabled) {
    background: var(--surface-3);
  }
  .btn:disabled {
    opacity: 0.6;
  }
  .btn.primary {
    background: var(--accent);
    border-color: var(--accent);
    color: var(--accent-ink);
  }
  .link {
    border: 0;
    background: none;
    padding: 0;
    color: var(--accent-text);
    font: inherit;
    font-size: 13px;
    font-weight: 600;
    cursor: pointer;
  }
  .link:hover {
    text-decoration: underline;
  }
</style>
