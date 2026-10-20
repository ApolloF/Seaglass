// Keeps a read-only copy of the real library (%APPDATA%\Seaglass\library.json)
// in OUT/real-library.json, for makeDevData's art and details. It never runs
// the app and never writes to %APPDATA%\Seaglass, which holds the synced
// Profile. To refresh the metadata, open Seaglass normally and run this again.
import fs from "node:fs";
import path from "node:path";
import { OUT } from "./lib.mjs";

const src = path.join(process.env.APPDATA, "Seaglass", "library.json");
if (!fs.existsSync(src)) {
  console.error(`no library at ${src}: open Seaglass once so it has one, then run this again`);
  process.exit(1);
}
const dest = path.join(OUT, "real-library.json");
fs.copyFileSync(src, dest);
const games = JSON.parse(fs.readFileSync(dest, "utf8")).games ?? [];
const done = games.filter((g) => !g.installed || (g.meta?.version ?? 0) >= 3).length;
console.log(`${done}/${games.length} games with current metadata`);
if (done < games.length) console.log("some are out of date: open Seaglass normally to refresh them, then run this again");
console.log("saved", dest);
