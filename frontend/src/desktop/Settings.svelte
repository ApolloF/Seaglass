<script lang="ts">
  import Icon from "../components/Icon.svelte";
  import Toggle from "../components/Toggle.svelte";
  import AccountsSettings from "./AccountsSettings.svelte";
  import SyncerSettings from "./SyncerSettings.svelte";
  import UpdateStatus from "./UpdateStatus.svelte";
  import { api } from "../lib/api";
  import { lib } from "../lib/store.svelte";
  import type { Settings, Startup } from "../lib/types";

  let { onclose }: { onclose: () => void } = $props();

  type Tab = "general" | "library" | "bigpicture" | "saves" | "about";
  let tab = $state<Tab>("library");
  const tabs: { id: Tab; label: string }[] = [
    { id: "general", label: "General" },
    { id: "library", label: "Library" },
    { id: "bigpicture", label: "Big picture" },
    { id: "saves", label: "Saves" },
    { id: "about", label: "About" },
  ];
  const layouts: { id: Settings["bigPictureLayout"]; label: string; note: string }[] = [
    { id: "deck", label: "Deck", note: "Side rail and rows of games" },
    { id: "console", label: "Console", note: "One row over full-screen art" },
    { id: "orbit", label: "Orbit", note: "An animated honeycomb" },
  ];

  let autoFolders = $state<string[]>([]);
  let hasKey = $state(false);
  let keyDraft = $state("");
  let startup = $state<Startup | null>(null);
  $effect(() => {
    api.autoFolders().then((f) => (autoFolders = f));
    api.hasSteamGridDBKey().then((k) => (hasKey = k));
    api.startWithWindows().then((v) => (startup = v));
  });

  async function copyDiagnostics() {
    const ok = await lib.run(() => api.copyDiagnostics().then(() => true));
    if (ok) lib.toast("Diagnostics copied. Paste them into your bug report.");
  }

  async function setStartup(on: boolean) {
    const next = await lib.run(() => api.setStartWithWindows(on));
    if (next) startup = next;
  }

  async function saveKey(k: string) {
    const ok = await lib.run(() => api.setSteamGridDBKey(k.trim()).then(() => true));
    if (ok) {
      hasKey = !!k.trim();
      keyDraft = "";
      lib.toast(hasKey ? "SteamGridDB key saved. Fetching the missing art…" : "SteamGridDB key removed");
    }
  }

  const s = $derived(lib.settings);
  const set = (patch: Partial<Settings>) => s && lib.saveSettings({ ...s, ...patch });

  async function addFolder() {
    const next = await lib.run(() => api.addFolder());
    if (next) lib.settings = next;
  }
  async function removeFolder(p: string) {
    const next = await lib.run(() => api.removeFolder(p));
    if (next) lib.settings = next;
  }

  let dialog: HTMLDivElement | undefined = $state();
  $effect(() => {
    dialog?.focus();
  });
</script>

<div class="scrim" role="presentation" onclick={(e) => e.target === e.currentTarget && onclose()}>
  <div class="dialog" role="dialog" aria-modal="true" aria-label="Settings" tabindex="-1" bind:this={dialog} onkeydown={(e) => e.key === "Escape" && onclose()}>
    <nav class="tabs" aria-label="Settings sections">
      <span class="heading">Settings</span>
      {#each tabs as t (t.id)}
        <button type="button" class:on={tab === t.id} aria-current={tab === t.id ? "page" : undefined} onclick={() => (tab = t.id)}>{t.label}</button>
      {/each}
    </nav>

    <section class="content">
      <div class="top">
        <h2>{tabs.find((t) => t.id === tab)?.label}</h2>
        <button type="button" class="close" aria-label="Close settings" onclick={onclose}><Icon name="close" size={18} stroke={2.2} /></button>
      </div>

      {#if s}
        {#if tab === "general"}
          <div class="group">
            <span class="glabel">Appearance</span>
            <div class="seg" role="group" aria-label="Theme">
              {#each [["system", "Follow Windows"], ["dark", "Dark"], ["light", "Light"]] as [id, label] (id)}
                <button type="button" class:on={s.theme === id} aria-pressed={s.theme === id} onclick={() => set({ theme: id as Settings["theme"] })}>{label}</button>
              {/each}
            </div>
            <p class="hint">Applies to desktop mode. Big picture mode is always dark.</p>
          </div>
          <div class="group">
            <span class="glabel">Starting</span>
            <Toggle
              checked={startup?.on ?? false}
              disabled={!startup}
              title="Start with Windows"
              detail="Seaglass waits in the tray when you sign in, so the PS button opens it and your library is ready."
              onchange={setStartup}
            />
            {#if startup?.on && startup.disabledByUser}
              <p class="hint warn"><Icon name="warn" size={14} stroke={2} />Windows skips it: it's turned off in Task Manager's Startup apps. Turn it on there too.</p>
            {/if}
          </div>
          <div class="group">
            <span class="glabel">Updates</span>
            <Toggle
              checked={s.autoUpdate}
              title="Keep Seaglass up to date"
              detail="Checks GitHub twice a day, downloads new versions in the background and installs them the next time Seaglass starts."
              onchange={(v) => set({ autoUpdate: v })}
            />
            <UpdateStatus />
          </div>
        {:else if tab === "library"}
          <div class="group">
            <Toggle checked={s.autoFolders} title="Look in common game folders" detail="Games and repack folders on every drive." onchange={(v) => set({ autoFolders: v })} />
            {#if s.autoFolders && autoFolders.length}
              <ul class="paths auto">
                {#each autoFolders as f (f)}<li><Icon name="folder" size={16} /><span>{f}</span></li>{/each}
              </ul>
            {/if}
            <Toggle checked={s.detectUnofficial} title="Recognise unofficial copies" detail="Steam emulators, cracks and repacks, matched to the right game by their Steam AppID." onchange={(v) => set({ detectUnofficial: v })} />
            <Toggle checked={s.reviewUncertain} title="Let me check uncertain matches" detail="Games matched only by folder name wait in New on this PC." onchange={(v) => set({ reviewUncertain: v })} />
            <Toggle checked={s.showNotInstalled} title="Show games you uninstalled" detail="They stay listed with their playtime." onchange={(v) => set({ showNotInstalled: v })} />
          </div>
          <div class="group">
            <span class="glabel">Libraries</span>
            {#each lib.libraries as l (l.id)}
              <Toggle checked={!l.hidden} title={`${l.label} games`} detail={`${l.count} ${l.count === 1 ? "game" : "games"}. Off: they aren't shown anywhere, but still count their playtime.`} onchange={() => lib.toggleLibrary(l.id)} />
            {/each}
          </div>
          <AccountsSettings />
          <div class="group">
            <span class="glabel">Your game folders</span>
            <p class="hint">Every folder inside these counts as a game.</p>
            {#if s.folders.length}
              <ul class="paths">
                {#each s.folders as f (f)}
                  <li>
                    <Icon name="folder" size={16} /><span>{f}</span>
                    <button type="button" class="icon" aria-label={`Stop looking in ${f}`} onclick={() => removeFolder(f)}><Icon name="close" size={14} stroke={2.2} /></button>
                  </li>
                {/each}
              </ul>
            {/if}
            <button type="button" class="btn" onclick={addFolder}><Icon name="plus" size={16} stroke={2.2} />Add folder</button>
          </div>
          <div class="group">
            <span class="glabel">Art for games Steam doesn't know</span>
            <p class="hint">
              Steam and GOG art needs no setup. For everything else, Seaglass can use SteamGridDB with your own free API key
              (steamgriddb.com, Preferences, API). The key is stored encrypted for your Windows account.
            </p>
            {#if hasKey}
              <div class="keyrow">
                <span class="ok"><Icon name="check" size={16} stroke={2.4} />SteamGridDB key saved</span>
                <button type="button" class="btn" onclick={() => saveKey("")}>Remove</button>
              </div>
            {:else}
              <form
                class="keyrow"
                onsubmit={(e) => {
                  e.preventDefault();
                  if (keyDraft.trim()) saveKey(keyDraft);
                }}
              >
                <label class="sr-only" for="sgdb-key">SteamGridDB API key</label>
                <input id="sgdb-key" class="key" type="password" autocomplete="off" spellcheck="false" placeholder="Paste your SteamGridDB API key" bind:value={keyDraft} />
                <button type="submit" class="btn" disabled={!keyDraft.trim()}>Save</button>
              </form>
            {/if}
          </div>
        {:else if tab === "bigpicture"}
          <div class="group">
            <span class="glabel">Layout</span>
            <div class="layouts">
              {#each layouts as l (l.id)}
                <button type="button" class="layout" class:on={s.bigPictureLayout === l.id} aria-pressed={s.bigPictureLayout === l.id} onclick={() => set({ bigPictureLayout: l.id })}>
                  <span class="thumb {l.id}" aria-hidden="true">
                    {#if l.id === "deck"}<i class="rail"></i><i class="a"></i><i class="b"></i><i class="c"></i><i class="d"></i><i class="e"></i>
                    {:else if l.id === "console"}<i class="t1"></i><i class="t2"></i><i class="t3"></i><i class="t4"></i><i class="bar"></i><i class="btn"></i>
                    {:else}<i class="o0"></i><i class="o1"></i><i class="o2"></i><i class="o3"></i><i class="o4"></i><i class="o5"></i><i class="o6"></i>{/if}
                  </span>
                  <span class="lname">{l.label}{#if l.id === "deck"}<span class="def">Default</span>{/if}</span>
                  <span class="lnote">{l.note}</span>
                </button>
              {/each}
            </div>
            <div class="seg" role="group" aria-label="Button prompts">
              {#each [["auto", "Prompts: auto"], ["playstation", "PlayStation"], ["xbox", "Xbox"]] as [id, label] (id)}
                <button type="button" class:on={s.glyphs === id} aria-pressed={s.glyphs === id} onclick={() => set({ glyphs: id as Settings["glyphs"] })}>{label}</button>
              {/each}
            </div>
          </div>
          <div class="group">
            <Toggle checked={s.openBigPictureOnController} title="Open big picture when a controller connects" detail="Switches over as soon as you pick one up." onchange={(v) => set({ openBigPictureOnController: v })} />
            <Toggle checked={s.startInBigPicture} title="Start in big picture" detail="Skip the desktop window when Seaglass starts." onchange={(v) => set({ startInBigPicture: v })} />
            <Toggle checked={s.psButton} title="PS / Xbox button opens Seaglass" detail="From other apps, and the overlay while you play. Turn off Steam's own guide-button shortcut to avoid both opening." onchange={(v) => set({ psButton: v })} />
            <Toggle checked={s.haptics} title="Haptics while browsing" detail="Light ticks as you move between games." onchange={(v) => set({ haptics: v })} />
            <Toggle checked={s.lightbar} title="Lightbar follows the game" detail="Tints the DualSense to the selected game's colour." onchange={(v) => set({ lightbar: v })} />
            <Toggle checked={s.sounds} title="Navigation sounds" detail="Soft clicks as you move." onchange={(v) => set({ sounds: v })} />
          </div>
          <p class="hint">Open big picture with the button in the sidebar, F11, or the PS / Xbox button.</p>
          <div class="group">
            <span class="glabel">While playing</span>
            <Toggle checked={s.closeWhilePlaying} title="Close the interface while a game runs" detail="Frees the memory it uses (about 400 MB). It comes back when the game exits; the tray icon opens it sooner." onchange={(v) => set({ closeWhilePlaying: v })} />
            <Toggle checked={s.noticeExternal} title="Notice games started elsewhere" detail="A game from your library started from Steam or a desktop shortcut counts its playtime here too, and Seaglass lets go of the controller while it runs." onchange={(v) => set({ noticeExternal: v })} />
            <Toggle checked={s.padWhilePlaying === "listen"} title="Listen for the PS / Xbox button in games" detail="Opens the overlay. Seaglass only listens: it never writes to the controller or changes its mode while a game has it. Turn off to let go of the controller completely." onchange={(v) => set({ padWhilePlaying: v ? "listen" : "off" })} />
          </div>
        {:else if tab === "saves"}
          <SyncerSettings />
        {:else}
          <dl class="kv">
            <dt>Version</dt>
            <dd>{lib.info?.version ?? ""}</dd>
            <dt>Updates</dt>
            <dd><UpdateStatus /></dd>
            <dt>Game database</dt>
            <dd>{lib.scan.known ? `${lib.scan.known.toLocaleString()} titles (Ludusavi manifest)` : "Downloading…"}</dd>
            <dt>Last scan</dt>
            <dd>{lib.scan.games} games in {(lib.scan.tookMs / 1000).toFixed(1)} s</dd>
            <dt>Data folder</dt>
            <dd class="path">{lib.info?.dataDir ?? ""}</dd>
          </dl>
          <div class="row">
            <button type="button" class="btn" onclick={copyDiagnostics}><Icon name="file" size={16} />Copy diagnostics</button>
            <button type="button" class="btn" onclick={() => lib.run(() => api.reportProblem())}><Icon name="link" size={16} />Report a problem</button>
            <button type="button" class="btn" onclick={() => lib.run(() => api.openLog())}><Icon name="folder" size={16} />Open log folder</button>
          </div>
          <p class="hint">Diagnostics are versions, settings, library counts and the end of the log, for a bug report. They hold no keys, and your user folder is shortened to %USERPROFILE%.</p>
        {/if}
      {/if}
    </section>
  </div>
</div>

<style>
  .scrim {
    position: fixed;
    inset: 0;
    z-index: 50;
    background: var(--scrim);
    display: flex;
    align-items: center;
    justify-content: center;
    animation: fade 0.18s ease both;
  }
  @keyframes fade {
    from {
      opacity: 0;
    }
  }
  .dialog {
    width: min(960px, calc(100vw - 64px));
    height: min(660px, calc(100vh - 96px));
    display: flex;
    border-radius: var(--radius-l);
    background: var(--surface);
    border: 1px solid var(--line-strong);
    box-shadow: var(--shadow);
    overflow: hidden;
    outline: none;
    animation: pop 0.22s var(--ease) both;
  }
  @keyframes pop {
    from {
      transform: scale(0.98);
      opacity: 0;
    }
  }
  .tabs {
    width: 210px;
    flex-shrink: 0;
    display: flex;
    flex-direction: column;
    gap: 2px;
    padding: 20px 12px;
    background: var(--bg);
    border-right: 1px solid var(--line);
  }
  .heading {
    padding: 0 12px 12px;
    font-family: var(--font-display);
    font-size: 26px;
    font-weight: 700;
  }
  .tabs button {
    height: 40px;
    padding: 0 12px;
    border: 0;
    border-radius: 10px;
    background: transparent;
    color: var(--text-2);
    font-size: 15px;
    font-weight: 600;
    text-align: left;
  }
  .tabs button:hover {
    background: var(--surface-2);
  }
  .tabs button.on {
    background: var(--accent-soft);
    color: var(--accent-text);
  }
  .content {
    flex: 1;
    min-width: 0;
    display: flex;
    flex-direction: column;
    gap: 18px;
    padding: 22px 28px 28px;
    overflow-y: auto;
  }
  .top {
    display: flex;
    align-items: center;
  }
  h2 {
    flex: 1;
    margin: 0;
    font-family: var(--font-display);
    font-size: 26px;
    font-weight: 700;
  }
  .close {
    width: 36px;
    height: 36px;
    border: 0;
    border-radius: 10px;
    background: var(--surface-2);
    display: flex;
    align-items: center;
    justify-content: center;
  }
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
  .hint {
    margin: -4px 0 2px;
    font-size: 13.5px;
    color: var(--muted);
  }
  .hint.warn {
    display: flex;
    align-items: center;
    gap: 6px;
    margin: 0;
    color: var(--warn);
  }
  .seg {
    display: flex;
    gap: 2px;
    padding: 3px;
    width: fit-content;
    border-radius: 10px;
    background: var(--surface-2);
  }
  .seg button {
    height: 34px;
    padding: 0 14px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    font-size: 14px;
    font-weight: 700;
  }
  .seg button.on {
    background: var(--surface-3);
    color: var(--text);
  }
  .paths {
    list-style: none;
    margin: 0;
    padding: 0;
    display: flex;
    flex-direction: column;
    gap: 6px;
  }
  .paths li {
    display: flex;
    align-items: center;
    gap: 10px;
    min-height: 38px;
    padding: 0 6px 0 12px;
    border-radius: 10px;
    background: var(--surface-2);
    color: var(--text-2);
    font-size: 14px;
  }
  .paths.auto li {
    background: transparent;
    min-height: 28px;
    color: var(--muted);
  }
  .paths span {
    flex: 1;
    min-width: 0;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }
  .icon {
    width: 30px;
    height: 30px;
    border: 0;
    border-radius: 8px;
    background: transparent;
    color: var(--muted);
    display: flex;
    align-items: center;
    justify-content: center;
  }
  .icon:hover {
    background: var(--surface-3);
    color: var(--text);
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
  .row {
    display: flex;
    flex-wrap: wrap;
    gap: 10px;
  }
  .keyrow {
    display: flex;
    align-items: center;
    gap: 10px;
  }
  .key {
    flex: 1;
    height: 38px;
    padding: 0 12px;
    border-radius: 10px;
    border: 1px solid var(--line-strong);
    background: var(--surface-2);
    outline: none;
    font-size: 14.5px;
  }
  .key:focus {
    border-color: var(--accent);
  }
  .ok {
    display: flex;
    align-items: center;
    gap: 8px;
    color: var(--accent-text);
    font-weight: 600;
  }
  .layouts {
    display: grid;
    grid-template-columns: repeat(3, minmax(0, 1fr));
    gap: 14px;
  }
  .layout {
    display: flex;
    flex-direction: column;
    gap: 6px;
    padding: 10px;
    border: 0;
    border-radius: 14px;
    background: var(--surface-2);
    text-align: left;
    box-shadow: 0 0 0 1px var(--line);
  }
  .layout.on {
    box-shadow: 0 0 0 3px var(--accent);
  }
  .thumb {
    position: relative;
    aspect-ratio: 16 / 9;
    border-radius: 8px;
    overflow: hidden;
    background: #0a0e13;
  }
  .thumb.console {
    background: radial-gradient(120% 120% at 70% 30%, oklch(0.55 0.12 40), oklch(0.2 0.05 20));
  }
  .thumb.orbit {
    background: #000;
  }
  .thumb i {
    position: absolute;
    border-radius: 3px;
    background: rgba(255, 255, 255, 0.35);
  }
  .deck .rail { left: 0; top: 0; bottom: 0; width: 8%; border-radius: 0; background: #1a2430; }
  .deck .a { left: 14%; top: 12%; width: 30%; height: 30%; background: #fff; }
  .deck .b { left: 47%; top: 12%; width: 30%; height: 30%; }
  .deck .c { left: 80%; top: 12%; width: 30%; height: 30%; }
  .deck .d { left: 14%; top: 52%; width: 22%; height: 22%; }
  .deck .e { left: 39%; top: 52%; width: 22%; height: 22%; }
  .console .t1 { left: 7%; top: 12%; width: 24%; height: 24%; background: #fff; }
  .console .t2 { left: 34%; top: 12%; width: 11%; height: 18%; }
  .console .t3 { left: 48%; top: 12%; width: 11%; height: 18%; }
  .console .t4 { left: 62%; top: 12%; width: 11%; height: 18%; }
  .console .bar { left: 7%; top: 56%; width: 38%; height: 9%; background: #fff; }
  .console .btn { left: 7%; top: 72%; width: 18%; height: 11%; border-radius: 99px; background: #fff; }
  .orbit i { border-radius: 50%; transform: translate(-50%, -50%); aspect-ratio: 1; }
  .orbit .o0 { left: 50%; top: 48%; width: 17%; background: #fff; }
  .orbit .o1 { left: 34%; top: 48%; width: 11%; background: oklch(0.7 0.12 30); }
  .orbit .o2 { left: 66%; top: 48%; width: 11%; background: oklch(0.7 0.12 250); }
  .orbit .o3 { left: 42%; top: 24%; width: 10%; background: oklch(0.7 0.12 150); }
  .orbit .o4 { left: 58%; top: 24%; width: 10%; background: oklch(0.7 0.12 320); }
  .orbit .o5 { left: 42%; top: 72%; width: 10%; background: oklch(0.7 0.12 200); }
  .orbit .o6 { left: 58%; top: 72%; width: 10%; background: oklch(0.7 0.12 90); }
  .lname {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 0 4px;
    font-weight: 700;
    font-size: 16px;
  }
  .def {
    font-size: 12px;
    font-weight: 700;
    color: var(--muted);
  }
  .lnote {
    padding: 0 4px 4px;
    font-size: 13px;
    color: var(--muted);
  }
  .kv {
    display: grid;
    grid-template-columns: 140px minmax(0, 1fr);
    gap: 10px 16px;
    margin: 0;
    font-size: 14.5px;
  }
  dt {
    color: var(--muted);
  }
  dd {
    margin: 0;
  }
  .path {
    word-break: break-all;
    user-select: text;
    -webkit-user-select: text;
  }
</style>
