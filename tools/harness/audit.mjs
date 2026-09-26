// Layout checks run inside the page: text that overlaps other text, text
// cut off by the screen or by its container, and text that overflows its
// box without an ellipsis. Only text that is actually on top (hit-tested)
// counts, so a screen under an open sheet doesn't report.
export function auditPage() {
  const W = innerWidth;
  const H = innerHeight;
  const issues = [];
  const boxes = [];
  const walker = document.createTreeWalker(document.body, NodeFilter.SHOW_TEXT);
  const name = (el) => {
    const c = typeof el.className === "string" ? el.className.split(" ").filter((x) => x && !x.startsWith("svelte-")).join(".") : "";
    return el.tagName.toLowerCase() + (c ? "." + c : "");
  };
  const onTop = (el, r) => {
    const x = Math.min(W - 1, Math.max(0, r.left + r.width / 2));
    const y = Math.min(H - 1, Math.max(0, r.top + r.height / 2));
    const hit = document.elementFromPoint(x, y);
    if (!hit) return false;
    if (hit === el || el.contains(hit) || hit.contains(el)) return true;
    // Hit something transparent to input over it? Treat pointer-events:none layers as see-through.
    return false;
  };
  while (walker.nextNode()) {
    const t = walker.currentNode;
    const text = t.textContent.trim();
    if (!text) continue;
    const el = t.parentElement;
    if (!el || !el.checkVisibility({ opacityProperty: true, visibilityProperty: true })) continue;
    let op = 1;
    for (let a = el; a; a = a.parentElement) op *= Number(getComputedStyle(a).opacity);
    if (op < 0.2) continue;
    const range = document.createRange();
    range.selectNodeContents(t);
    for (const r of range.getClientRects()) {
      if (r.width < 2 || r.height < 2) continue;
      if (r.right <= 0 || r.bottom <= 0 || r.left >= W || r.top >= H) continue; // off screen on purpose (scrolled rows)
      boxes.push({ el, text: text.slice(0, 40), r });
    }
  }
  // Inside something that scrolls, being cut at its edge is how scrolling looks.
  const scrolls = (el) => {
    for (let a = el.parentElement; a && a !== document.body; a = a.parentElement) {
      const cs = getComputedStyle(a);
      if (/auto|scroll/.test(cs.overflowY + cs.overflowX) && (a.scrollHeight > a.clientHeight + 1 || a.scrollWidth > a.clientWidth + 1)) return true;
    }
    return false;
  };
  const visible = boxes.filter((b) => onTop(b.el, b.r) || getComputedStyle(b.el).pointerEvents === "none");
  for (const b of visible) {
    const { r } = b;
    if (scrolls(b.el)) continue;
    if (r.left < -1 || r.top < -1 || r.right > W + 1 || r.bottom > H + 1) {
      issues.push({ kind: "cut by screen", text: b.text, el: name(b.el), rect: [r.left, r.top, r.right, r.bottom].map(Math.round) });
      continue;
    }
    // Cut by a container that hides overflow.
    for (let a = b.el.parentElement; a && a !== document.body; a = a.parentElement) {
      const cs = getComputedStyle(a);
      if (cs.overflowX === "visible" && cs.overflowY === "visible") continue;
      const c = a.getBoundingClientRect();
      const inside = r.left >= c.left - 1 && r.right <= c.right + 1 && r.top >= c.top - 1 && r.bottom <= c.bottom + 1;
      const outside = r.right <= c.left || r.left >= c.right || r.bottom <= c.top || r.top >= c.bottom;
      if (!inside && !outside) {
        issues.push({ kind: "cut by container", text: b.text, el: name(b.el), by: name(a), rect: [r.left, r.top, r.right, r.bottom].map(Math.round) });
      }
      break;
    }
  }
  // Text that overflows its own box with no ellipsis.
  const seen = new Set();
  for (const b of visible) {
    if (seen.has(b.el)) continue;
    seen.add(b.el);
    const cs = getComputedStyle(b.el);
    if (cs.overflowX !== "visible" && b.el.scrollWidth > b.el.clientWidth + 1 && cs.textOverflow !== "ellipsis" && !/auto|scroll/.test(cs.overflowX)) {
      issues.push({ kind: "overflows box", text: b.text, el: name(b.el) });
    }
  }
  // Text over other text.
  for (let i = 0; i < visible.length; i++) {
    for (let j = i + 1; j < visible.length; j++) {
      const a = visible[i];
      const c = visible[j];
      if (a.el === c.el || a.el.contains(c.el) || c.el.contains(a.el)) continue;
      const x = Math.min(a.r.right, c.r.right) - Math.max(a.r.left, c.r.left);
      const y = Math.min(a.r.bottom, c.r.bottom) - Math.max(a.r.top, c.r.top);
      if (x <= 2 || y <= 2) continue;
      const small = Math.min(a.r.width * a.r.height, c.r.width * c.r.height);
      if ((x * y) / small > 0.15) issues.push({ kind: "text overlaps text", text: `${a.text} ⟷ ${c.text}`, el: `${name(a.el)} ⟷ ${name(c.el)}` });
    }
  }
  return issues;
}
