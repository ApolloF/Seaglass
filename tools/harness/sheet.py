"""Contact sheet: several screenshots of one screen side by side, each
labelled, so all screen sizes can be looked at in one picture.

    python sheet.py out.png a.png b.png ...
"""
import sys
from PIL import Image, ImageDraw

out, files = sys.argv[1], sys.argv[2:]
cols = 3
cell_w = 900
thumbs = []
for f in files:
    im = Image.open(f).convert("RGB")
    h = round(im.height * cell_w / im.width)
    thumbs.append((f, im.resize((cell_w, h), Image.LANCZOS)))
rows = [thumbs[i:i + cols] for i in range(0, len(thumbs), cols)]
row_h = [max(t.height for _, t in r) + 28 for r in rows]
sheet = Image.new("RGB", (cols * cell_w + (cols - 1) * 8, sum(row_h) + 8 * (len(rows) - 1)), (40, 40, 40))
d = ImageDraw.Draw(sheet)
y = 0
for r, rh in zip(rows, row_h):
    x = 0
    for f, t in r:
        label = f.replace("\\", "/").rsplit("/", 1)[-1].rsplit(".", 1)[0]
        d.text((x + 6, y + 6), label, fill=(255, 255, 0))
        sheet.paste(t, (x, y + 28))
        x += cell_w + 8
    y += rh + 8
sheet.save(out)
