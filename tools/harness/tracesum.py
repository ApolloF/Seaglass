"""Sums a Chrome trace's complete events by thread and name: where the time went.

    python tracesum.py trace.json [top]
"""
import json, sys, collections
data = json.load(open(sys.argv[1], encoding="utf-8"))
events = data["traceEvents"] if isinstance(data, dict) else data
top = int(sys.argv[2]) if len(sys.argv) > 2 else 12
names = {}
for e in events:
    if e.get("ph") == "M" and e.get("name") == "thread_name":
        names[(e["pid"], e["tid"])] = e["args"]["name"]
tot = collections.defaultdict(float)
cnt = collections.Counter()
span = [1e30, 0]
for e in events:
    if e.get("ph") != "X" or "dur" not in e:
        continue
    th = names.get((e["pid"], e["tid"]), str(e["tid"]))
    k = (th, e["name"])
    tot[k] += e["dur"] / 1000
    cnt[k] += 1
    span[0] = min(span[0], e["ts"]); span[1] = max(span[1], e["ts"] + e["dur"])
print(f"trace span {(span[1]-span[0])/1e6:.1f} s")
by_thread = collections.defaultdict(list)
for (th, n), ms in tot.items():
    by_thread[th].append((ms, n, cnt[(th, n)]))
for th, lst in sorted(by_thread.items(), key=lambda x: -sum(m for m, _, _ in x[1])):
    lst.sort(reverse=True)
    if lst[0][0] < 20:
        continue
    print(f"\n{th}")
    for ms, n, c in lst[:top]:
        print(f"  {ms:9.1f} ms  {c:6d}×  {n}")
