// Achievements end to end in the real app: a fake external copy with a
// Goldberg-style emulator (Portal, Steam app 400, so Steam's global rarity
// is real), its schema and an icon in steam_settings, and its unlock file
// in %APPDATA%\GSE Saves. Opens its details, plays it, unlocks two more
// while it runs, and checks the note after it exits.
//
//   node achievements.mjs
//
// The unlock file that was there before (if any) is put back afterwards.
import fs from "node:fs";
import path from "node:path";
import { auditPage } from "./audit.mjs";
import { OUT, backupAppData, fakeGameExe, restoreAppData, sleep, startApp } from "./lib.mjs";
import { SIZES } from "./sizes.mjs";

const APP = 400;
const dir = path.join(OUT, "achievements");
fs.rmSync(dir, { recursive: true, force: true });
fs.mkdirSync(dir, { recursive: true });

// A 1×1 PNG: steam_settings icons are copied, checked and re-encoded.
const PNG = Buffer.from("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mNk+M9QDwADhgGAWjR9awAAAABJRU5ErkJggg==", "base64");

const saves = path.join(process.env.APPDATA, "GSE Saves", String(APP));
const unlockFile = path.join(saves, "achievements.json");
const unlockBackup = fs.existsSync(unlockFile) ? fs.readFileSync(unlockFile) : null;
const now = Math.floor(Date.now() / 1000);
const writeUnlocks = (ids) =>
  fs.writeFileSync(unlockFile, JSON.stringify(Object.fromEntries(ids.map((id, k) => [id, { earned: true, earned_time: now - 3600 + k }]))));

const checks = [];
const check = (what, ok, detail = "") => (checks.push({ what, ok, detail }), console.log(`${ok ? "ok  " : "FAIL"} ${what}${detail ? ` (${detail})` : ""}`));

backupAppData();
let app;
try {
  // The game: the fake game, set up as a Goldberg copy of Portal.
  const game = path.join(OUT, "games", "Goldberg");
  fs.rmSync(game, { recursive: true, force: true });
  fs.mkdirSync(path.join(game, "steam_settings", "achievement_images"), { recursive: true });
  fs.copyFileSync(fakeGameExe(), path.join(game, "FakeGame.exe"));
  fs.writeFileSync(path.join(game, "fakegame.txt"), "--run=12 --title=Fake_Goldberg");
  fs.writeFileSync(path.join(game, "steam_settings", "steam_appid.txt"), String(APP));
  fs.writeFileSync(path.join(game, "steam_settings", "achievement_images", "gun.png"), PNG);
  fs.writeFileSync(
    path.join(game, "steam_settings", "achievements.json"),
    JSON.stringify([
      { name: "PORTAL_GET_PORTALGUNS", displayName: "Lab Rat", description: "Get both portal devices.", hidden: "0", icon: "gun.png" },
      { name: "PORTAL_ESCAPE_TESTCHAMBERS", displayName: "Fratricide", description: "Escape the test chambers.", hidden: "0" },
      { name: "PORTAL_BEAT_GAME", displayName: "Heartbreaker", description: "Finish the game.", hidden: "1" },
    ]),
  );
  fs.mkdirSync(saves, { recursive: true });
  writeUnlocks(["PORTAL_GET_PORTALGUNS"]);

  const data = path.join(OUT, "data", "achievements");
  fs.rmSync(data, { recursive: true, force: true });
  fs.mkdirSync(data, { recursive: true });
  fs.writeFileSync(
    path.join(data, "library.json"),
    JSON.stringify({
      version: 1, nextId: 2,
      games: [{
        id: 1, key: game.toLowerCase(), title: "Fake Goldberg", sortTitle: "fake goldberg", source: "folder",
        sourceLabel: "External copy", external: true, emulator: "Goldberg", emuDir: ".", steamAppId: APP,
        installed: true, dir: game, exe: path.join(game, "FakeGame.exe"), workDir: game, how: "Game folder (test)",
        matchHow: "Test game", confidence: 100, needsReview: false, addedAt: now - 86400, initial: true, seenAt: now, padMode: "native",
      }],
    }),
  );
  fs.writeFileSync(path.join(data, "settings.json"), JSON.stringify({ autoFolders: false, autoUpdate: false, startSyncer: false, syncSavesBefore: false, backupSavesAfter: false, noticeExternal: false, welcomed: true }));

  app = await startApp({ data, pad: null });
  await app.viewport(1600, 1000);
  const page = app.page;
  await page.getByText("Fake Goldberg", { exact: true }).first().click();
  const card = page.locator(".card.ach");
  await card.waitFor({ timeout: 20000 });
  await page.waitForFunction(() => /\d+ \/ \d+/.test(document.querySelector(".card.ach")?.textContent ?? ""), null, { timeout: 30000 });
  const text = (await card.textContent()) ?? "";
  check("details card counts the unlock", text.includes("1 / 3"), text.replace(/\s+/g, " ").trim());
  const icons = await card.locator(".ach-recent img").evaluateAll((els) => els.map((e) => e.getAttribute("src")));
  check("the local icon is served from /ach/", icons.length === 1 && /^\/ach\/[0-9a-f]{40}\.png$/.test(icons[0] ?? ""), icons.join(", "));
  const loaded = await card.locator(".ach-recent img").first().evaluate((img) => img.complete && img.naturalWidth > 0).catch(() => false);
  check("the icon loads", loaded);
  await app.shot(path.join(dir, "1-details.png"));

  await card.getByRole("button", { name: "Show all" }).click();
  await page.locator(".dialog").waitFor();
  const rows = await page.locator(".dialog li").allTextContents();
  check("hidden achievement is masked", rows.some((r) => r.includes("Hidden achievement")) && !rows.some((r) => r.includes("Heartbreaker")));
  check("rarity from Steam", rows.some((r) => /% of players/.test(r)), rows.find((r) => /% of players/.test(r))?.replace(/\s+/g, " ") ?? "none (offline?)");
  await app.shot(path.join(dir, "2-dialog.png"));
  await page.keyboard.press("Escape");

  // Play; two more unlocks while it runs; the note after it exits.
  await page.getByRole("button", { name: "Play", exact: true }).click();
  await sleep(5000);
  writeUnlocks(["PORTAL_GET_PORTALGUNS", "PORTAL_ESCAPE_TESTCHAMBERS", "PORTAL_BEAT_GAME"]);
  const main = await (async () => {
    const end = Date.now() + 40000;
    while (Date.now() < end) {
      const s = await app.state().catch(() => null);
      if (s && ["ended", "failed", "cancelled"].includes(s.session?.phase)) break;
      await sleep(500);
    }
    return app.mainPage();
  })();
  const toast = await main
    .waitForFunction(() => [...document.querySelectorAll(".toast")].map((t) => t.textContent ?? "").find((t) => t.includes("achievements unlocked")) ?? false, null, { timeout: 20000 })
    .then((h) => h.jsonValue())
    .catch(() => "");
  check("note after playing", String(toast).includes("2 achievements unlocked"), String(toast).trim());
  await app.shot(path.join(dir, "3-after.png"), main);
  await main.waitForFunction(() => (document.querySelector(".card.ach")?.textContent ?? "").includes("3 / 3"), null, { timeout: 10000 }).catch(() => {});
  const after = (await main.locator(".card.ach").textContent().catch(() => "")) ?? "";
  check("card shows the new count", after.includes("3 / 3"), after.replace(/\s+/g, " ").trim());

  // Layout: the full list at every tour size, then big picture's screen
  // (the only game is the focused one: △ opens its sheet).
  const audit = async (name) => {
    const found = {};
    for (const [w, h, scale] of SIZES) {
      await app.viewport(w, h, scale, main);
      await sleep(400);
      await app.shot(path.join(dir, name, `${w}x${h}.png`), main);
      const issues = await main.evaluate(auditPage);
      if (issues.length) found[`${w}x${h}`] = issues;
    }
    check(`${name}: no layout issues`, !Object.keys(found).length, JSON.stringify(found).slice(0, 400));
  };
  await main.locator(".card.ach").getByRole("button", { name: "Show all" }).click();
  await main.locator(".dialog").waitFor();
  await audit("desktop-dialog");
  await main.keyboard.press("Escape");
  await app.viewport(1920, 1080, 1, main);
  await main.keyboard.press("F11");
  await sleep(3000);
  for (let k = 0; k < 5 && !(await main.locator(".sheet").count()); k++) {
    await main.keyboard.press("i");
    await sleep(800);
  }
  for (let k = 0; k < 8; k++) (await main.keyboard.press("ArrowRight"), await sleep(150));
  const label = await main.evaluate(() => document.querySelector(".buttons .btn.on")?.textContent?.trim() ?? "");
  check("big picture sheet has the achievements button", label.startsWith("Achievements"), label);
  if (label.startsWith("Achievements")) await main.keyboard.press("Enter"); // anything else might start the game
  await sleep(800);
  const bp = await main.evaluate(() => document.querySelector(".screen")?.textContent ?? "");
  check("big picture achievements screen", bp.includes("3 / 3"), bp.replace(/\s+/g, " ").slice(0, 80));
  await audit("bigpicture-screen");
} finally {
  await app?.quit();
  restoreAppData();
  if (unlockBackup) fs.writeFileSync(unlockFile, unlockBackup);
  else fs.rmSync(saves, { recursive: true, force: true });
}
fs.writeFileSync(path.join(dir, "results.json"), JSON.stringify(checks, null, 1));
const failed = checks.filter((c) => !c.ok);
console.log(`\n${checks.length - failed.length}/${checks.length} checks passed; screenshots in ${dir}`);
process.exitCode = failed.length ? 1 : 0;
