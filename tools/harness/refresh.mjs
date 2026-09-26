// Runs the real app once on the real library (backed up first) so its
// metadata and art are current, and keeps a copy of the refreshed library
// for makeDevData. The real %APPDATA%\WaterLauncher is restored after.
import fs from "node:fs";
import path from "node:path";
import { OUT, backupAppData, restoreAppData, sleep, startApp } from "./lib.mjs";

backupAppData();
let app;
try {
  app = await startApp({ pad: null });
  const lib = path.join(process.env.APPDATA, "WaterLauncher", "library.json");
  for (let k = 0; k < 90; k++) {
    await sleep(2000);
    const games = JSON.parse(fs.readFileSync(lib, "utf8")).games;
    const done = games.filter((g) => !g.installed || (g.meta?.version ?? 0) >= 3).length;
    process.stdout.write(`\r${done}/${games.length} games with current metadata`);
    if (done === games.length && k > 3) break;
  }
  await app.quit();
  fs.copyFileSync(path.join(process.env.APPDATA, "WaterLauncher", "library.json"), path.join(OUT, "real-library.json"));
  console.log("\nsaved", path.join(OUT, "real-library.json"));
} finally {
  await app?.quit();
  restoreAppData();
}
