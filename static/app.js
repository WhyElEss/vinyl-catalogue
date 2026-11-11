// The collection page. Records stand in stacks on a horizontal rail, like
// crates in a record shop:
//   ← →        move between stacks
//   ↓ Enter    in the overview: dig into the stack in the middle
//   ↓ ↑        while digging: flip through the records of that stack;
//              ↑ on the first record stops digging, like Esc
//   Enter      while digging: open the record at the front
//   Esc        close the record; then stop digging (back to the overview)
// Every gesture is one of these keys, on every system (see press()): a
// wheel, two fingers on a trackpad or one finger on a screen moving DOWN is
// ↓, UP is ↑; sideways they turn pages — moving LEFT is →, RIGHT is ←. One
// swipe is one key press. The server put the whole collection into
// <script id="data"> and the strings into <script id="texts">.
(() => {
  "use strict";
  const appSrc = document.currentScript ? document.currentScript.src : "/static/app.js";
  const $ = (s, el = document) => el.querySelector(s);
  const data = JSON.parse($("#data").textContent || "[]");
  const T = JSON.parse($("#texts").textContent || "{}");
  const stage = $("#stage");
  const toolbar = $("#toolbar");
  const dialog = $("#overlay");

  // ---- debug: ?debug=wheel ----
  // A panel in the corner lists every wheel event (time since the previous
  // one, deltaX, deltaY) and every key press with where it came from; "Save"
  // downloads the whole log as JSON. Only for finding out what a browser
  // really sends; nothing leaves the page.
  const DEBUG = new URLSearchParams(location.search).get("debug") === "wheel";
  const dbg = { t0: performance.now(), log: [], lines: [], panel: null, list: null, count: null, queued: false, lastWheel: 0 };
  const now = () => Math.round((performance.now() - dbg.t0) * 10) / 10;
  function dlog(entry, line) {
    if (!DEBUG) return;
    dbg.log.push(entry);
    if (line) {
      dbg.lines.push(line);
      if (dbg.lines.length > 18) dbg.lines.shift();
    }
    if (dbg.queued) return;
    dbg.queued = true;
    requestAnimationFrame(() => {
      dbg.queued = false;
      const w = dbg.log.filter((x) => x[0] === "w").length;
      const k = dbg.log.filter((x) => x[0] === "k").length;
      dbg.count.textContent = "wheel events: " + w + " · presses: " + k;
      dbg.list.textContent = dbg.lines.join("\n");
    });
  }
  if (DEBUG) {
    const panel = document.createElement("div");
    panel.className = "debug-panel";
    const head = document.createElement("div");
    head.className = "debug-head";
    dbg.count = document.createElement("span");
    const save = document.createElement("button");
    save.type = "button";
    save.textContent = "Save log";
    save.addEventListener("click", () => {
      const blob = new Blob([JSON.stringify({ ua: navigator.userAgent, saved: new Date().toISOString(), log: dbg.log })], { type: "application/json" });
      const a = document.createElement("a");
      a.href = URL.createObjectURL(blob);
      a.download = "vinyl-debug-log.json";
      document.body.appendChild(a);
      a.click();
      a.remove();
    });
    // Copy puts the log on the clipboard (execCommand works on plain http,
    // where navigator.clipboard does not); if even that fails, the log is
    // shown selected so that ⌘C copies it.
    const copy = document.createElement("button");
    copy.type = "button";
    copy.textContent = "Copy log";
    copy.addEventListener("click", () => {
      const text = JSON.stringify({ ua: navigator.userAgent, saved: new Date().toISOString(), log: dbg.log });
      const ta = document.createElement("textarea");
      ta.className = "debug-text";
      ta.value = text;
      panel.appendChild(ta);
      ta.select();
      let ok = false;
      try {
        ok = document.execCommand("copy");
      } catch (e) {}
      if (ok) {
        ta.remove();
        copy.textContent = "Copied " + Math.round(text.length / 1024) + " KB";
      } else {
        copy.textContent = "Press ⌘C";
      }
      setTimeout(() => (copy.textContent = "Copy log"), 4000);
    });
    const clear = document.createElement("button");
    clear.type = "button";
    clear.textContent = "Clear";
    clear.addEventListener("click", () => {
      dbg.log = [];
      dbg.lines = [];
      dbg.t0 = performance.now();
      dlog(["c", 0]);
    });
    head.append(dbg.count, copy, save, clear);
    // Test copies (DEBUG_LOG=1 on the server) also keep the log there: it is
    // sent every 2 s while it grows. Elsewhere the post gets a 404, once.
    // A page opened with b=<digits> in its address posts under that number
    // (debug-wheel-log-<b>.json), so other open test pages cannot overwrite
    // its log; the address loses b on the first syncURL, so it is read now.
    const pageNo = new URLSearchParams(location.search).get("b") || "";
    const logNo = /^\d+$/.test(pageNo) ? "?b=" + pageNo : "";
    let sent = 0, sending = false, stop = false;
    setInterval(() => {
      if (stop || sending || dbg.log.length === sent) return;
      sending = true;
      const n = dbg.log.length;
      fetch("/debug/wheel-log" + logNo, {
        method: "POST",
        headers: { "Content-Type": "application/json" },
        body: JSON.stringify({ ua: navigator.userAgent, saved: new Date().toISOString(), log: dbg.log }),
      })
        .then((r) => {
          if (r.status === 404) stop = true;
          else if (r.ok) sent = n;
          dbg.count.dataset.sent = r.ok ? "sent " + n : "";
        })
        .catch(() => {})
        .finally(() => (sending = false));
    }, 2000);
    dbg.list = document.createElement("pre");
    panel.append(head, dbg.list);
    document.body.appendChild(panel);
    dbg.panel = panel;
    dlog(["c", 0], "debug on — wheel: gap ms, dx, dy");
    // Every frame that came more than 25 ms after the previous one (a
    // dropped frame), whatever caused it.
    let last = performance.now();
    const beat = (t) => {
      if (t - last > 25) dlog(["j", Math.round(t - dbg.t0), Math.round(t - last)]);
      last = t;
      requestAnimationFrame(beat);
    };
    requestAnimationFrame(beat);
  }
  const dialogBody = $("#overlay-body");
  if (!data.length) return;

  toolbar.hidden = false;
  const fallback = $("#fallback");
  if (fallback) fallback.remove();

  const GROUPS = ["artist", "genre", "decade", "year"];
  const params = new URLSearchParams(location.search);
  const state = {
    group: GROUPS.includes(params.get("g")) ? params.get("g") : "artist",
    view: params.get("v") === "wall" ? "wall" : "stacks",
    q: params.get("q") || "",
    open: params.get("s"), // key of the stack being dug through, or null
    pos: Math.max(0, parseInt(params.get("r") || "0", 10) || 0), // its front record
    at: params.get("at"), // stack in the middle of the overview
    desc: params.get("o") === "desc", // reverse order of stacks and records
  };
  const baseTitle = document.title;

  // ---- helpers ----
  const el = (tag, cls, text) => {
    const e = document.createElement(tag);
    if (cls) e.className = cls;
    if (text != null) e.textContent = text;
    return e;
  };
  const icon = (d) => {
    const ns = "http://www.w3.org/2000/svg";
    const s = document.createElementNS(ns, "svg");
    s.setAttribute("viewBox", "0 0 24 24");
    s.setAttribute("aria-hidden", "true");
    const p = document.createElementNS(ns, "path");
    p.setAttribute("d", d);
    s.appendChild(p);
    return s;
  };
  const LEFT = "M15 18l-6-6 6-6";
  const RIGHT = "M9 18l6-6-6-6";
  const UP = "M6 15l6-6 6 6";
  const DOWN = "M6 9l6 6 6-6";
  const button = (cls, label, path, onClick) => {
    const b = el("button", cls);
    b.type = "button";
    b.setAttribute("aria-label", label);
    b.title = label;
    b.appendChild(icon(path));
    b.addEventListener("click", (e) => {
      e.stopPropagation();
      onClick();
    });
    return b;
  };
  const plural = (n) => n + " " + (n === 1 ? T.one : T.many);
  const fold = (s) => s.toLowerCase().normalize("NFD").replace(/\p{M}/gu, "").replace(/ё/g, "е");
  const collator = new Intl.Collator("en", { sensitivity: "base", numeric: true });
  data.forEach((r) => {
    r.sy = r.o || r.y || 0; // original year, else this pressing's until the master is read
    r.hay = fold([r.a, r.t, r.q, r.f, r.y, r.o].join(" "));
    r.k = fold(r.s || "");
  });

  // Artist initial: Latin letters first, then Cyrillic, then "#".
  const initial = (k) => {
    const c = k.charAt(0);
    if (/[a-z]/.test(c)) return { key: c.toUpperCase(), rank: 0 };
    if (/[а-я]/.test(c)) return { key: c.toUpperCase(), rank: 1 };
    return { key: "#", rank: 2 };
  };
  // The server's order (p): artist without "The", then the original year,
  // then the title or the owner's "Sort as"; compilations last, by title.
  const byArtist = (a, b) => a.p - b.p;

  function groups(list) {
    const m = new Map();
    const add = (key, label, rank, r) => {
      if (!m.has(key)) m.set(key, { key, label, rank, items: [] });
      m.get(key).items.push(r);
    };
    for (const r of list) {
      switch (state.group) {
        case "artist": {
          // Compilations stand apart, in one stack after "#", as on the shelf.
          if (r.v) {
            add("various", T.various, 3, r);
            break;
          }
          const i = initial(r.k);
          add(i.key, i.key, i.rank, r);
          break;
        }
        case "genre":
          add(r.g || "~", r.g || T.other, r.g ? 0 : 1, r);
          break;
        case "decade":
          if (!r.sy) add("~", T.other, 1, r);
          else {
            const d = Math.floor(r.sy / 10) * 10;
            add(String(d), d + "s", 0, r);
          }
          break;
        case "year":
          if (!r.sy) add("~", T.other, 1, r);
          else add(String(r.sy), String(r.sy), 0, r);
          break;
      }
    }
    const gs = [...m.values()];
    gs.forEach((g) => g.items.sort(byArtist));
    gs.sort((a, b) => {
      if (a.rank !== b.rank) return a.rank - b.rank;
      if (state.group === "decade" || state.group === "year") return Number(a.key) - Number(b.key);
      return collator.compare(a.key, b.key);
    });
    if (state.desc) {
      // Reverse order: the stacks and the records in each one.
      gs.reverse();
      gs.forEach((g) => g.items.reverse());
    }
    return gs;
  }

  function filtered() {
    const words = fold(state.q).split(/\s+/).filter(Boolean);
    if (!words.length) return data;
    return data.filter((r) => words.every((w) => r.hay.includes(w)));
  }

  let listURL = location.pathname === "/" ? location.pathname + location.search : "/";
  function syncURL() {
    const p = new URLSearchParams();
    if (state.group !== "artist") p.set("g", state.group);
    if (state.desc) p.set("o", "desc");
    if (state.view !== "stacks") p.set("v", state.view);
    if (state.q) p.set("q", state.q);
    if (state.view === "stacks" && gs[center]) {
      if (state.open) {
        p.set("s", state.open);
        if (state.pos) p.set("r", String(state.pos));
      } else if (center > 0) p.set("at", gs[center].key);
    }
    if (DEBUG) p.set("debug", "wheel");
    const qs = p.toString();
    const url = "/" + (qs ? "?" + qs : "");
    if (location.pathname === "/" && location.pathname + location.search !== url) {
      history.replaceState(history.state, "", url);
    }
    listURL = url;
  }

  // ---- rendering ----
  let gs = []; // stacks shown
  let rail = null;
  let slots = [];
  let center = -1;
  let strip = null;
  let prevBtn = null, nextBtn = null;

  function render(focusKey) {
    stage.textContent = "";
    rail = null;
    slots = [];
    center = -1;
    const list = filtered();
    if (!list.length) {
      gs = [];
      stage.appendChild(el("p", "empty", T.nothing));
      syncURL();
      return;
    }
    gs = groups(list);
    if (state.view === "wall") {
      renderWall();
      syncURL();
      return;
    }
    if (state.open && !gs.some((g) => g.key === state.open)) state.open = null;
    renderRail(focusKey || state.open || state.at);
    state.at = null;
  }

  // how: true = load now; "defer" = no source yet (a pile far from the
  // middle; showNear() gives it one when the middle comes close); otherwise
  // the browser's own lazy loading (the wall).
  // Only the covers of the piles within NEAR of the middle are fetched: the
  // rail is far wider than the screen, and the browser's lazy loading counts
  // thousands of pixels sideways as "near" (PageSpeed: 29 piles' covers
  // fetched with 3 on screen).
  const NEAR = 3;
  function showNear() {
    for (let i = Math.max(0, center - NEAR); i <= Math.min(slots.length - 1, center + NEAR); i++) {
      const img = slots[i].querySelector(".rec img");
      if (img) {
        img.loading = "eager";
        if (img.dataset.src) {
          img.src = img.dataset.src;
          delete img.dataset.src;
        }
      }
      buildLid(i);
    }
    lidPump(); // and the lids further ahead, in spare moments
  }

  // ---- the crate lid ----
  // A closed stack wears a front lid of smoked plexiglass over its first
  // cover (owner's design, 2026-11-11): the stack's label (letter, decade,
  // year, VARIOUS) and a finger slot are cut through it, so the cover shows
  // sharp inside them; the cut has a polished edge and, inside, the shadow
  // of the plexiglass's thickness. Entering the stack drops the lid forward.
  // The label is set in Jost 700; as no stencil face with Cyrillic exists
  // under a free licence (checked 2026-11-11), bridges are added where a
  // piece of plexiglass would otherwise fall out (inside O, A, B, R, Ю, Ф,
  // 8, # …), found by rasterising the cut.
  const LID_FONT = '700 X "Jost"';
  const LID_SLOT = "M 115 50 L 185 50 A 13 13 0 0 0 185 24 L 115 24 A 13 13 0 0 0 115 50 Z";
  const fontReady = document.fonts ? document.fonts.load(LID_FONT.replace("X", "100px"), "AБ0").catch(() => {}) : Promise.resolve();
  const lidCache = new Map();
  const lidText = (g) => (g.key === "various" ? "VARIOUS" : g.label);

  // lidGeometry: font size, baseline and bridges for a label, in the lid's
  // 300 × 300 units. A label of one character is set large; longer ones are
  // fitted into 82% of the lid's width.
  // Measured in Safari 27.2 (owner's Mac, 2026-11-11): at 2× this took
  // 91–130 ms per label on the main thread, and every long frame of the row
  // coincided with one; so it runs at 1× (a quarter of the pixels), only
  // while the row stands still (lidPump), and is remembered in the browser.
  const GEO_KEY = "lidgeo1:";
  function lidGeometry(label) {
    if (lidCache.has(label)) return lidCache.get(label);
    try {
      const saved = localStorage.getItem(GEO_KEY + label);
      if (saved) {
        const geo = JSON.parse(saved);
        lidCache.set(label, geo);
        return geo;
      }
    } catch (e) {}
    const tg = performance.now();
    const S = 1, W = 300 * S;
    const cv = document.createElement("canvas");
    cv.width = cv.height = W;
    const c = cv.getContext("2d");
    // Every label is set at one size (single letters 190, longer labels
    // fitted into 82% of the lid's width) and centred by its real outline
    // under the finger slot, so a J or Q with a tail sits at the same height
    // as the others instead of on the baseline.
    const ink = (sz) => {
      c.font = LID_FONT.replace("X", sz + "px");
      const m = c.measureText(label);
      return { w: m.actualBoundingBoxLeft + m.actualBoundingBoxRight, a: m.actualBoundingBoxAscent, d: m.actualBoundingBoxDescent };
    };
    let size = [...label].length === 1 ? 190 : 120;
    let m = ink(size);
    if (m.w > 246) size *= 246 / m.w;
    m = ink(size);
    const y = 180 - (m.d - m.a) / 2;
    c.fillStyle = "#000";
    c.fillRect(0, 0, W, W);
    c.fillStyle = "#fff";
    c.font = LID_FONT.replace("X", size * S + "px");
    c.textAlign = "center";
    c.fillText(label, 150 * S, y * S);
    const px = c.getImageData(0, 0, W, W).data;
    const cut = new Uint8Array(W * W);
    for (let i = 0; i < W * W; i++) cut[i] = px[i * 4] > 127 ? 1 : 0;
    // The plexiglass reachable from the lid's border stays on; whatever is
    // not reached is an island and needs a bridge.
    const outside = new Uint8Array(W * W);
    const st = [];
    for (let x = 0; x < W; x++) st.push(x, (W - 1) * W + x);
    for (let yy = 0; yy < W; yy++) st.push(yy * W, yy * W + W - 1);
    while (st.length) {
      const i = st.pop();
      if (outside[i] || cut[i]) continue;
      outside[i] = 1;
      const x = i % W;
      if (x > 0) st.push(i - 1);
      if (x < W - 1) st.push(i + 1);
      if (i >= W) st.push(i - W);
      if (i < W * (W - 1)) st.push(i + W);
    }
    const bridges = [];
    const bw = Math.max(3, size * 0.035);
    const done = new Uint8Array(W * W);
    for (let i = 0; i < W * W; i++) {
      if (cut[i] || outside[i] || done[i]) continue;
      const pix = [];
      const s2 = [i];
      while (s2.length) {
        const j = s2.pop();
        if (cut[j] || outside[j] || done[j]) continue;
        done[j] = 1;
        pix.push(j);
        const x = j % W;
        if (x > 0) s2.push(j - 1);
        if (x < W - 1) s2.push(j + 1);
        if (j >= W) s2.push(j - W);
        if (j < W * (W - 1)) s2.push(j + W);
      }
      if (pix.length < 6) continue; // a speck, not a piece
      let minX = W, maxX = 0;
      for (const j of pix) {
        const x = j % W;
        if (x < minX) minX = x;
        if (x > maxX) maxX = x;
      }
      const cx = Math.round((minX + maxX) / 2);
      const col = pix.filter((j) => j % W === cx).map((j) => (j / W) | 0);
      if (!col.length) continue;
      const top = Math.min(...col), bot = Math.max(...col);
      let a = top - 1;
      while (a > 0 && cut[a * W + cx]) a--;
      let b = bot + 1;
      while (b < W - 1 && cut[b * W + cx]) b++;
      const up = { x: cx / S - bw / 2, y: a / S, w: bw, h: (top - a) / S + 0.5 };
      const down = { x: cx / S - bw / 2, y: bot / S - 0.5, w: bw, h: (b - bot) / S + 0.5 };
      // Two bridges, like a stencil's O — but where one would run much
      // longer than the other (the apex of A), only the short one.
      if (up.h > down.h * 1.4) bridges.push(down);
      else if (down.h > up.h * 1.4) bridges.push(up);
      else bridges.push(up, down);
    }
    const geo = { size, y, bridges };
    lidCache.set(label, geo);
    try {
      localStorage.setItem(GEO_KEY + label, JSON.stringify(geo));
    } catch (e) {}
    if (DEBUG) dlog(["lg", Math.round(tg - dbg.t0), label, Math.round(performance.now() - tg)], "lid geometry " + label + ": " + Math.round(performance.now() - tg) + " ms");
    return geo;
  }

  // The lid is drawn straight onto a canvas, and that canvas stands in the
  // lid: a finished bitmap, which the browser only copies while the row
  // moves (a live SVG with blur and masks was redrawn on every paint — the
  // row jerked on the owner's Mac). Until 2026-11-11 the lid was an SVG
  // picture turned into a JPEG; in Chromium rasterising that SVG took
  // ~12 ms of the ~16 ms per lid (the JPEG encode itself 2–4 ms), PageSpeed
  // counted 20 long tasks, and every new <img> became the page's LCP.
  // A canvas is no LCP candidate, so the cover under it stays the LCP.
  const LID_PX = 600, LU = LID_PX / 300; // canvas pixels per lid unit
  // The plexiglass's blur (Gaussian, σ 7 units) is computed by the page,
  // not with ctx.filter: Safari 27.2 accepts ctx.filter = "blur(…)" (it
  // reads back) but drew the cover sharp (owner's screenshot, 2026-11-11).
  // The cover is blurred at an eighth of the size, where σ is 1.75 px, by
  // three box blurs in a worker (static/lidblur.js), then enlarged.
  // Against Chromium's own Gaussian on 8 covers: mean difference 0.3–2.2 of
  // 255, at most 21. In the owner's Safari the arithmetic took ~50 ms a lid
  // at a quarter of the size and 14–20 ms at an eighth on the main thread
  // (0.5–6 ms in Chromium) — hence the worker.
  const BLUR_K = 8, BLUR_S = LID_PX / BLUR_K, BLUR_SIGMA = (7 * LU) / BLUR_K;
  const BLUR_PAD = Math.ceil(3 * BLUR_SIGMA), BLUR_W = BLUR_S + 2 * BLUR_PAD;
  const BLUR_BOXES = (() => {
    const n = 3, sg = BLUR_SIGMA;
    let wl = Math.floor(Math.sqrt((12 * sg * sg) / n + 1));
    if (wl % 2 === 0) wl--;
    const m = Math.round((12 * sg * sg - n * wl * wl - 4 * n * wl - 3 * n) / (-4 * wl - 4));
    return [0, 1, 2].map((i) => (i < m ? wl : wl + 2));
  })();
  let blurWorker, blurSeq = 0;
  const blurJobs = new Map();
  function lidWorker() {
    if (blurWorker === undefined) {
      try {
        blurWorker = new Worker(appSrc.replace(/app\.js/, "lidblur.js"));
        blurWorker.onmessage = (e) => {
          const done = blurJobs.get(e.data.id);
          blurJobs.delete(e.data.id);
          if (done) done(e.data.buf, e.data.ms);
        };
        blurWorker.onerror = () => {
          // no blur at all rather than no lids
          blurWorker = null;
          blurJobs.forEach((done) => done(null));
          blurJobs.clear();
        };
      } catch (e) {
        blurWorker = null;
      }
    }
    return blurWorker;
  }
  // Covers decoded together (two seconds after load nearly all of them)
  // are handed to the worker one per task: preparing each takes a few ms on
  // the main thread, and 25 at once made an 84 ms frame in Safari.
  const blurQueue = [];
  function queueBlur(c) {
    blurQueue.push(c);
    if (blurQueue.length === 1) setTimeout(blurNext, 0);
  }
  function blurNext() {
    blurCover(blurQueue.shift());
    if (blurQueue.length) setTimeout(blurNext, 0);
  }
  // blurCover makes the blurred cover of c (a small canvas, c.blur) and
  // then marks c ready; without a worker c is ready without a blur.
  function blurCover(c) {
    const w = lidWorker();
    if (!w) {
      c.st = "ready";
      return;
    }
    const W = BLUR_W, img = c.img;
    const cv = document.createElement("canvas");
    cv.width = cv.height = W;
    const x = cv.getContext("2d", { willReadFrequently: true });
    x.imageSmoothingQuality = "high";
    const iw = img.naturalWidth, ih = img.naturalHeight, s = Math.min(iw, ih);
    x.drawImage(img, (iw - s) / 2, (ih - s) / 2, s, s, BLUR_PAD, BLUR_PAD, BLUR_S, BLUR_S);
    const data = x.getImageData(0, 0, W, W).data;
    const id = ++blurSeq;
    blurJobs.set(id, (buf, ms) => {
      if (buf) {
        x.putImageData(new ImageData(new Uint8ClampedArray(buf), W, W), 0, 0);
        c.blur = cv;
        if (DEBUG) dlog(["bt", Math.round(performance.now() - dbg.t0), Math.round(ms * 10) / 10]);
      }
      c.st = "ready";
    });
    w.postMessage({ id, W, boxes: BLUR_BOXES, buf: data.buffer }, [data.buffer]);
  }
  const scratch = [];
  function pad(n) {
    if (!scratch[n]) {
      scratch[n] = document.createElement("canvas");
      scratch[n].width = scratch[n].height = LID_PX;
    }
    const c = scratch[n].getContext("2d");
    c.setTransform(1, 0, 0, 1, 0, 0);
    c.globalCompositeOperation = "source-over";
    c.globalAlpha = 1;
    c.clearRect(0, 0, LID_PX, LID_PX);
    return c;
  }
  // The first cover of each pile, decoded once for its lid (the same URL as
  // the cover on the rail, so the browser's cache serves it).
  const lidCovers = new Map();
  function lidCover(g) {
    const url = g.items[0] && g.items[0].c;
    if (!url) return { st: "ready", img: null, blur: null };
    let c = lidCovers.get(url);
    if (!c) {
      const img = new Image();
      img.decoding = "async";
      img.src = url;
      c = { st: "loading", img, blur: null };
      img.decode().then(() => queueBlur(c), () => {
        c.st = "ready"; // a lid without its cover rather than none
        c.img = null;
      });
      lidCovers.set(url, c);
    }
    return c;
  }
  // The cover, cut to a square around its middle (SVG "xMidYMid slice").
  function drawSlice(c, img) {
    const w = img.naturalWidth, h = img.naturalHeight, s = Math.min(w, h);
    c.drawImage(img, (w - s) / 2, (h - s) / 2, s, s, 0, 0, LID_PX, LID_PX);
  }
  function setLabel(c, size) {
    c.font = LID_FONT.replace("X", size + "px");
    c.textAlign = "center";
  }

  // drawLid paints the lid of stack g over its cover (lidCover: the image
  // and its blurred copy, either may be missing).
  // All lengths are in lid units (300 across), as the design was drawn.
  function drawLid(g, cover) {
    const img = cover.img;
    const label = lidText(g);
    const { size, y, bridges } = lidGeometry(label);
    const slot = new Path2D(LID_SLOT);
    // The cut: the label and the finger slot, minus the bridges.
    const k = pad(0);
    k.scale(LU, LU);
    setLabel(k, size);
    k.fillText(label, 150, y);
    k.fill(slot);
    k.globalCompositeOperation = "destination-out";
    for (const b of bridges) k.fillRect(b.x, b.y, b.w, b.h);
    const cut = scratch[0];

    const cv = document.createElement("canvas");
    cv.width = cv.height = LID_PX;
    cv.setAttribute("aria-hidden", "true");
    const c = cv.getContext("2d");
    c.fillStyle = "#2a2b2e";
    c.fillRect(0, 0, LID_PX, LID_PX);
    if (img) drawSlice(c, img); // sharp, seen through the cut

    // Smoked plexiglass everywhere but the cut: the cover blurred
    // (Gaussian, 7 units), darkened.
    const f = pad(1);
    if (cover.blur) {
      f.imageSmoothingQuality = "high";
      f.drawImage(cover.blur, BLUR_PAD, BLUR_PAD, BLUR_S, BLUR_S, 0, 0, LID_PX, LID_PX);
    } else if (img) drawSlice(f, img); // no worker: clear plexiglass
    f.fillStyle = "rgba(30,32,36,.5)";
    f.fillRect(0, 0, LID_PX, LID_PX);
    f.globalCompositeOperation = "destination-out";
    f.drawImage(cut, 0, 0);
    c.drawImage(scratch[1], 0, 0);

    // The plexiglass's thickness, inside the cut: the plexiglass (which
    // goes on above the lid's top edge) moved down 1…10 units, .0875 each.
    const p = pad(2);
    p.fillRect(0, 0, LID_PX, LID_PX);
    p.globalCompositeOperation = "destination-out";
    p.drawImage(cut, 0, 0);
    const s = pad(3);
    s.globalAlpha = 0.0875;
    for (let d = 1; d <= 10; d++) {
      s.drawImage(scratch[2], 0, d * LU);
      s.fillRect(0, 0, LID_PX, d * LU);
    }
    s.globalAlpha = 1;
    s.globalCompositeOperation = "destination-in";
    s.drawImage(cut, 0, 0);
    c.drawImage(scratch[3], 0, 0);

    // The polished edge: a band 0.8 units wide just inside the cut's
    // outline — the cut minus the cut shrunk by 0.8 units (as the SVG's
    // erode did). Shrunk = the cut kept only where it also covers itself
    // moved 0.8 units each way. Not a stroke of the letters: a glyph may
    // be drawn from overlapping outlines (Jost's A), whose inner lines a
    // stroke would show.
    const e = pad(2), o = 0.8 * LU;
    e.drawImage(cut, 0, 0);
    e.globalCompositeOperation = "destination-in";
    for (const [dx, dy] of [[o, 0], [-o, 0], [0, o], [0, -o], [o, o], [o, -o], [-o, o], [-o, -o]]) e.drawImage(cut, dx, dy);
    const r = pad(3);
    r.drawImage(cut, 0, 0);
    r.globalCompositeOperation = "destination-out";
    r.drawImage(scratch[2], 0, 0);
    r.globalCompositeOperation = "source-in";
    r.fillStyle = "#fff";
    r.fillRect(0, 0, LID_PX, LID_PX);
    c.globalAlpha = 0.95;
    c.drawImage(scratch[3], 0, 0);
    c.globalAlpha = 1;

    // A gloss from the top left, and the lid's edge.
    const gl = c.createLinearGradient(0, 0, LID_PX, LID_PX);
    gl.addColorStop(0, "rgba(255,255,255,.28)");
    gl.addColorStop(0.45, "rgba(255,255,255,0)");
    gl.addColorStop(1, "rgba(255,255,255,.08)");
    c.fillStyle = gl;
    c.fillRect(0, 0, LID_PX, LID_PX);
    c.scale(LU, LU);
    c.beginPath();
    c.roundRect(0.75, 0.75, 298.5, 298.5, 3);
    c.lineWidth = 1.5;
    c.strokeStyle = "rgba(255,255,255,.55)";
    c.stroke();
    return cv;
  }

  // Lid work happens one piece at a time and never while the row moves:
  // first the lids of the piles near the middle, then, in spare moments,
  // the bridges of every other label of this grouping.
  const lidWanted = new Set();
  let lidPumping = false;
  function buildLid(i) {
    const s = slots[i];
    const lid = s && s.firstChild.querySelector(".lid");
    if (!lid || lid.dataset.built) return;
    lidWanted.add(i);
    lidPump();
  }
  function lidPump() {
    if (lidPumping) return;
    lidPumping = true;
    fontReady.then(() => setTimeout(lidStep, 0));
  }
  // Lids are made ahead of the middle so a pile never arrives as a black
  // square (owner, 2026-11-11, with keys held the row hardly stops): while
  // the row stands still, every unmade lid, nearest first; while it
  // moves, only one that is about to be seen, at most every 150 ms.
  // Until the page has loaded and rested, only the piles within NEAR, so
  // the first visit fetches no more covers than before. A lid is drawn
  // only once its cover has arrived; the covers of the lids still to make
  // are fetched meanwhile.
  let lidWide = false, lidLast = 0;
  window.addEventListener("load", () => setTimeout(() => {
    lidWide = true;
    lidPump();
  }, 2000));
  function lidStep() {
    const reach = lidWide ? slots.length : NEAR;
    let best = -1, loading = 0;
    if (center >= 0) {
      for (let d = 0; d <= reach && best < 0; d++) {
        for (const i of d ? [center - d, center + d] : [center]) {
          const lid = slots[i] && slots[i].firstChild.querySelector(".lid");
          if (!lid || lid.dataset.built || lid.hidden) continue;
          if (lidCover(gs[i]).st === "ready") {
            best = i;
            break;
          }
          loading++;
        }
      }
    }
    lidWanted.clear();
    if (best >= 0) {
      const moving = railMoving();
      const urgent = Math.abs(best - center) <= NEAR + 1;
      if (moving && (!urgent || performance.now() - lidLast < 150)) {
        setTimeout(lidStep, 50);
        return;
      }
      lidLast = performance.now();
      makeLid(best);
      setTimeout(lidStep, moving ? 50 : 16);
      return;
    }
    if (loading || railMoving()) {
      setTimeout(lidStep, 50);
      return;
    }
    // nothing to make: work out one more label's bridges ahead of time
    const next = gs.find((g) => !lidCache.has(lidText(g)));
    if (next) {
      lidGeometry(lidText(next));
      setTimeout(lidStep, 40);
      return;
    }
    lidPumping = false;
  }

  function makeLid(i) {
    const lid = slots[i] && slots[i].firstChild.querySelector(".lid");
    if (!lid || lid.dataset.built) return;
    lid.dataset.built = "1";
    const g = gs[i];
    const t = performance.now();
    lid.replaceChildren(drawLid(g, lidCover(g)));
    if (DEBUG) dlog(["lb", Math.round(t - dbg.t0), g.key, Math.round((performance.now() - t) * 10) / 10], "lid " + g.key + ": " + Math.round(performance.now() - t) + " ms");
  }

  // paintLid puts the lid on pile i (closed) or takes it off (open). With
  // animate the lid drops forward on its bottom edge, or comes back up.
  const LID_MS = 420;
  function paintLid(i, open, animate) {
    const s = slots[i];
    if (!s) return;
    const pile = s.firstChild;
    let lid = pile.querySelector(".lid");
    if (!lid) {
      lid = el("div", "lid");
      pile.appendChild(lid);
      // Built when the stack comes near the middle (showNear()).
      if (center >= 0 && Math.abs(i - center) <= NEAR) buildLid(i);
    }
    lid.getAnimations().forEach((a) => a.cancel());
    const shut = [{ transform: "perspective(1100px) rotateX(0deg)", opacity: 1 }];
    const down = [{ transform: "perspective(1100px) rotateX(-96deg)", opacity: 0 }];
    if (open) {
      if (animate && !reduceMotion.matches && lid.animate && !lid.hidden) {
        lid.animate([...shut, ...down], { duration: LID_MS, easing: "cubic-bezier(.5,0,.75,.4)" }).onfinish = () => {
          if (state.open === gs[i].key) lid.hidden = true;
        };
      } else lid.hidden = true;
    } else {
      const was = lid.hidden;
      lid.hidden = false;
      if (animate && was && !reduceMotion.matches && lid.animate) {
        lid.animate([...down, ...shut], { duration: LID_MS, easing: "cubic-bezier(.2,.7,.3,1)" });
      }
    }
  }
  function cover(r, how) {
    if (!r.c) {
      const none = el("span", "nocover");
      none.dataset.i = r.i;
      return none;
    }
    const img = el("img");
    img.dataset.i = r.i; // the card opens from this cover and closes into it
    if (how === "defer") img.dataset.src = r.c;
    else img.src = r.c;
    img.alt = "";
    img.loading = how === true ? "eager" : "lazy";
    img.decoding = "async";
    img.width = img.height = 600;
    return img;
  }

  // Sleeve edges behind the front record: none for a single record.
  const edges = (behind) => (behind > 0 ? Math.min(behind, 40) * 1.5 + 2 + "px" : "0px");

  function renderRail(focusKey) {
    const wrap = el("div", "rail-wrap");
    rail = el("div", "rail" + (state.open ? " is-dig" : ""));
    prevBtn = button("nav-btn prev", T.prev, LEFT, () => go(from() - 1));
    nextBtn = button("nav-btn next", T.next, RIGHT, () => go(from() + 1));
    wrap.append(prevBtn, rail, nextBtn);
    stage.appendChild(wrap);

    gs.forEach((g, i) => {
      const s = el("div", "slot");
      s.dataset.key = g.key;
      const pile = el("div", "pile");
      const front = el("div", "rec");
      pile.appendChild(front);
      const label = el("div", "slot-label");
      s.append(pile, label);
      s.addEventListener("click", (e) => onSlotClick(i, e));
      rail.appendChild(s);
      slots.push(s);
      paintSlot(i, g.key === state.open ? state.pos : 0);
    });

    strip = el("nav", "index-strip");
    gs.forEach((g, i) => {
      const b = el("button", "", g.label);
      b.type = "button";
      b.addEventListener("click", () => go(i));
      strip.appendChild(b);
    });
    stage.appendChild(strip);

    rail.addEventListener("scroll", onScroll, { passive: true });
    rail.addEventListener("wheel", onWheel, { passive: false });
    rail.addEventListener("touchstart", onTouchStart, { passive: true });
    rail.addEventListener("touchend", onTouchEnd, { passive: true });

    const start = Math.max(0, gs.findIndex((g) => g.key === focusKey));
    requestAnimationFrame(() => {
      go(start, true);
      updateCenter();
    });
  }

  // paintSlot draws stack i with record idx at the front.
  function paintSlot(i, idx) {
    const g = gs[i], s = slots[i];
    const n = g.items.length;
    idx = Math.max(0, Math.min(n - 1, idx));
    const r = g.items[idx];
    s.dataset.pos = idx;
    const pile = s.firstChild;
    pile.style.setProperty("--h", edges(n - idx - 1));
    pile.style.setProperty("--taper", (Math.min(n - idx - 1, 40) / 40).toFixed(3));
    const front = pile.querySelector(".rec");
    front.replaceChildren(cover(r, Math.abs(i - center) <= NEAR || "defer"));
    paintFallen(pile, g, g.key === state.open ? idx : 0);
    if (!pile.querySelector(".lid")) paintLid(i, g.key === state.open, false);
    const label = s.lastChild;
    label.textContent = "";
    if (g.key === state.open) {
      // Digging: the front record's artist and title, its place, and buttons.
      label.append(el("span", "slot-title rec-artist", r.a), el("span", "slot-sub", r.t));
      const row = el("div", "dig-row");
      const up = button("mini-btn", T.prev_record, UP, () => flip(-1));
      const down = button("mini-btn", T.next_record, DOWN, () => flip(1));
      up.disabled = idx === 0;
      down.disabled = idx === n - 1;
      row.append(up, el("span", "slot-count", idx + 1 + " " + T.of + " " + n), down);
      label.appendChild(row);
      s.setAttribute("aria-label", r.a + " — " + r.t + ", " + (idx + 1) + " " + T.of + " " + n);
    } else {
      // The lid already carries the stack's name; under it, the count.
      label.append(el("span", "slot-count", plural(n)));
      s.setAttribute("aria-label", g.label + ", " + plural(n));
    }
  }

  // The records already flipped lie in front of the pile, under the front
  // record: plain edges, as many as were flipped. None before the first flip.
  function paintFallen(pile, g, idx) {
    let f = pile.querySelector(".fallen");
    if (idx === 0) {
      if (f) f.remove();
      return;
    }
    if (!f) {
      f = el("div", "fallen");
      pile.appendChild(f);
    }
    f.style.setProperty("--t", edges(idx));
  }

  function onSlotClick(i, e) {
    if (e.target.closest("button")) return;
    if (i !== center) {
      go(i);
      return;
    }
    const g = gs[i];
    if (state.open === g.key) openRelease(g.items[state.pos].i);
    else enterStack(i);
  }

  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");

  function enterStack(i) {
    const g = gs[i];
    if (!g) return;
    state.open = g.key;
    state.pos = 0;
    rail.classList.add("is-dig");
    paintSlot(i, 0);
    paintLid(i, true, true);
    syncURL();
  }

  function leaveStack() {
    if (!state.open) return;
    const i = gs.findIndex((g) => g.key === state.open);
    state.open = null;
    state.pos = 0;
    rail.classList.remove("is-dig");
    if (i >= 0) {
      paintSlot(i, 0);
      paintLid(i, false, true);
    }
    syncURL();
  }

  // flip moves the front of the middle stack by dir (+1 deeper, -1 back).
  function flip(dir) {
    if (center < 0) return;
    const g = gs[center];
    if (state.open !== g.key) return; // a stack is entered explicitly (Enter, a click)
    const n = g.items.length;
    const next = state.pos + dir;
    const pile = slots[center].firstChild;
    if (next < 0 || next >= n) {
      pile.classList.remove("bump");
      void pile.offsetWidth;
      pile.classList.add("bump");
      return;
    }
    // A flip still running is cut short, so quick flips never stack up.
    pile.querySelectorAll(".rec.ghost").forEach((x) => x.remove());
    const front = pile.querySelector(".rec:not(.ghost)");
    front.classList.remove("rise", "hold");
    // Going back is going forward played backwards, with the same length
    // and mirrored easing: the record that fell comes back up over the
    // front one (tilt-in), while the front one settles back into the pile
    // (sink). The real front stays hidden until the returning record is up.
    const ghosts = [];
    const ghost = (cls, child) => {
      const x = el("div", "rec ghost " + cls);
      x.appendChild(child);
      pile.appendChild(x);
      ghosts.push(x);
      return x;
    };
    if (dir > 0) {
      ghost("tilt-out", cover(g.items[state.pos], true)); // the front record falls forward
    } else {
      ghost("sink", cover(g.items[state.pos], true)); // the front record goes back into the pile
      ghost("tilt-in", cover(g.items[next], true)); // the fallen one comes back up
    }
    const done = () => {
      ghosts.forEach((x) => x.remove());
      front.classList.remove("hold");
    };
    ghosts[ghosts.length - 1].addEventListener("animationend", done, { once: true });
    setTimeout(done, 700);
    state.pos = next;
    paintSlot(center, next);
    void front.offsetWidth;
    front.classList.add(dir > 0 ? "rise" : "hold");
    syncURL();
  }

  function renderWall() {
    for (const g of gs) {
      const sec = el("section", "wall-group");
      const h = el("h2", "", g.label);
      h.appendChild(el("span", "", plural(g.items.length)));
      const grid = el("div", "wall");
      for (const r of g.items) {
        const a = el("a", "card");
        a.href = "/release/" + r.i;
        a.append(cover(r), el("span", "card-a", r.a), el("span", "card-t", r.t));
        a.addEventListener("click", (e) => {
          if (e.metaKey || e.ctrlKey || e.shiftKey || e.button !== 0) return;
          e.preventDefault();
          openRelease(r.i);
        });
        grid.appendChild(a);
      }
      sec.append(h, grid);
      stage.appendChild(sec);
    }
  }

  // ---- moving along the rail ----
  // aim is the stack a smooth scroll is heading for, until the middle
  // reaches it, so quick presses or swipes count from there: a second → while
  // the rail still moves goes one stack further instead of being lost.
  let aim = -1;
  const from = () => (aim >= 0 ? aim : center);
  // The rail is moved by our own frame-by-frame animation, not the
  // browser's smooth scrolling: Safari cancels a smooth scrollTo as soon as
  // trackpad events arrive over the rail (even ones the page cancels), and
  // the rail then crept back or stopped short (measured with ?debug=wheel,
  // 2026-11-10). Nothing else scrolls the rail, so it needs no scroll-snap.
  const RAIL_MS = 380;
  const railMove = { raf: 0, until: 0 };
  const railMoving = () => performance.now() < railMove.until;
  function slideTo(left) {
    cancelAnimationFrame(railMove.raf);
    const from = rail.scrollLeft, dist = left - from;
    if (Math.abs(dist) < 1 || reduceMotion.matches) {
      rail.scrollLeft = left;
      return;
    }
    const t0 = performance.now();
    railMove.until = t0 + RAIL_MS + 60;
    const ease = (t) => 1 - Math.pow(1 - t, 3); // ease-out, like the flips
    const r = rail;
    let prevFrame = t0;
    const step = (now) => {
      if (DEBUG) {
        dlog(["f", Math.round(now - dbg.t0), Math.round((now - prevFrame) * 10) / 10]);
        prevFrame = now;
      }
      const t = Math.min(1, (now - t0) / RAIL_MS);
      r.scrollLeft = from + dist * ease(t);
      if (t < 1 && r === rail) railMove.raf = requestAnimationFrame(step);
    };
    railMove.raf = requestAnimationFrame(step);
  }
  function go(i, instant) {
    if (!rail || !slots.length) return;
    i = Math.max(0, Math.min(slots.length - 1, i));
    aim = instant || i === center ? -1 : i;
    if (DEBUG) dlog(["g", now(), center, i, instant ? 1 : 0]);
    const s = slots[i];
    const left = s.offsetLeft + s.offsetWidth / 2 - rail.clientWidth / 2;
    if (instant) {
      cancelAnimationFrame(railMove.raf);
      rail.scrollLeft = left;
      updateCenter();
    } else slideTo(left);
  }

  let ticking = false;
  function onScroll() {
    if (DEBUG) dlog(["s", now(), Math.round(rail.scrollLeft)]);
    if (ticking) return;
    ticking = true;
    requestAnimationFrame(() => {
      ticking = false;
      updateCenter();
    });
  }

  function updateCenter() {
    if (!rail || !slots.length) return;
    const mid = rail.scrollLeft + rail.clientWidth / 2;
    let best = 0, dist = Infinity;
    slots.forEach((s, i) => {
      const d = Math.abs(s.offsetLeft + s.offsetWidth / 2 - mid);
      if (d < dist) {
        dist = d;
        best = i;
      }
    });
    if (best === aim) aim = -1;
    if (best === center) return;
    const prev = center;
    if (slots[prev]) slots[prev].classList.remove("is-center");
    center = best;
    slots[center].classList.add("is-center");
    // Moving away from an open stack closes it; the stack that comes to the
    // middle stays closed (owner, 2026-11-11).
    if (state.open && gs[center].key !== state.open) leaveStack();
    showNear();
    prevBtn.disabled = center === 0;
    nextBtn.disabled = center === slots.length - 1;
    [...strip.children].forEach((b, i) => b.setAttribute("aria-current", i === center ? "true" : "false"));
    syncURL();
  }

  // press is the one place where keys and gestures turn into actions.
  let pressFrom = "key";
  function press(key) {
    if (!rail || center < 0) return false;
    if (DEBUG) {
      dlog(["k", now(), key, pressFrom, center, aim], "PRESS " + key + " (" + pressFrom + ")");
      pressFrom = "key";
    }
    const g = gs[center];
    const digging = state.open === g.key;
    switch (key) {
      case "ArrowRight":
        if (digging) leaveStack(); // close it at once, before the row moves
        go(from() + 1);
        break;
      case "ArrowLeft":
        if (digging) leaveStack();
        go(from() - 1);
        break;
      case "ArrowDown":
        if (digging) flip(1);
        else enterStack(center); // ↓ in the overview digs into the middle stack
        break;
      case "ArrowUp":
        // ↑ on the first record has nowhere to flip back to: it leaves the
        // stack, as Esc does (↓ came in, ↑ goes out).
        if (digging && state.pos > 0) flip(-1);
        else if (digging) leaveStack();
        break;
      case "Enter":
        if (digging) openRelease(g.items[state.pos].i);
        else enterStack(center);
        break;
      case "Escape":
        if (!state.open) return false;
        leaveStack();
        break;
      default:
        return false;
    }
    return true;
  }

  // Wheel and trackpad: every swipe is one arrow key, in both directions —
  // the rail never scrolls by itself.
  //
  // Direction. Down and up move the way the hand does, like the ↓ key:
  // fingers or wheel down = the next record. Apple devices scroll
  // "naturally" by default, so a downward swipe arrives as a negative
  // deltaY there; elsewhere a wheel turned down gives a positive one (the
  // browser cannot tell whether natural scrolling is switched off).
  // Sideways works like turning a page, as on a phone: fingers moving left
  // bring the next pile (→); that arrives as a positive deltaX.
  //
  // One step per swipe. A trackpad sends a swipe and then its inertia as one
  // stream of events about 8 ms apart. Recorded on the owner's Mac (Safari,
  // 2026-11-10): a new swipe — even a fast one landing on the previous
  // swipe's inertia — always begins after a break of 27–39 ms in the stream,
  // while one swipe and its inertia never pause for 20 ms until their last
  // 1–2 px. So a break of WHEEL_GAP ms starts a new gesture, and a gesture
  // steps once, along its stronger axis, as soon as it has moved WHEEL_STEP
  // px. On that recording this gives exactly one step per swipe (10/10 slow,
  // 10/10 fast, 10/10 up, 5+5 mixed, 10/10 sideways), each at the start of
  // the swipe.
  const NATURAL = /Mac|iPhone|iPad|iPod/i.test((navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "");
  //
  // Right after a press the page is busy starting its animation, and Safari
  // then holds the trackpad events for 30–60 ms and hands them over as one
  // big delta — a break that looks like a new swipe. On the owner's debug
  // log every such false second press came 18–79 ms after the first, and
  // the quickest real swipes 440 ms apart; so no new swipe starts within
  // WHEEL_AFTER_PRESS ms of a press (100–250 ms all gave the same result on
  // both recordings).
  const WHEEL_STEP = 30, WHEEL_GAP = 22, WHEEL_AFTER_PRESS = 150;
  const wheel = { x: 0, y: 0, last: -Infinity, done: false, pressed: -Infinity };
  function onWheel(e) {
    e.preventDefault();
    const k = e.deltaMode === 1 ? 16 : e.deltaMode === 2 ? rail.clientHeight : 1; // lines, pages → pixels
    const now = e.timeStamp || performance.now();
    if (DEBUG) {
      const gap = wheel.last > -Infinity ? Math.round(now - wheel.last) : -1;
      dlog(["w", Math.round((performance.now() - dbg.t0) * 10) / 10, Math.round((now - dbg.t0) * 10) / 10, e.deltaX, e.deltaY, e.deltaMode, gap], "w +" + gap + "ms  " + e.deltaX + ", " + e.deltaY);
    }
    if (now - wheel.last >= WHEEL_GAP && now - wheel.pressed >= WHEEL_AFTER_PRESS) {
      wheel.x = 0;
      wheel.y = 0;
      wheel.done = false;
    }
    wheel.last = now;
    wheel.x += e.deltaX * k;
    wheel.y += e.deltaY * k;
    if (wheel.done) return;
    const side = Math.abs(wheel.x) > Math.abs(wheel.y);
    const moved = side ? wheel.x : wheel.y;
    if (Math.abs(moved) < WHEEL_STEP) return;
    wheel.done = true;
    wheel.pressed = now;
    pressFrom = "wheel";
    if (side) press(moved > 0 ? "ArrowRight" : "ArrowLeft");
    else press((NATURAL ? -moved : moved) > 0 ? "ArrowDown" : "ArrowUp");
  }

  // Touch: every swipe is one arrow-key press — finger down = ↓, up = ↑;
  // sideways the finger pushes the row like a page: moving left brings the
  // next pile (→), moving right the previous one (←). The rail itself does
  // not follow the finger (touch-action in style.css).
  let touch = null;
  function onTouchStart(e) {
    const t = e.changedTouches[0];
    touch = { x: t.clientX, y: t.clientY };
  }
  function onTouchEnd(e) {
    if (!touch) return;
    const t = e.changedTouches[0];
    const dx = t.clientX - touch.x, dy = t.clientY - touch.y;
    touch = null;
    const ax = Math.abs(dx), ay = Math.abs(dy);
    if (ay > 40 && ay > ax * 1.5) press(dy > 0 ? "ArrowDown" : "ArrowUp");
    else if (ax > 40 && ax > ay * 1.5) press(dx < 0 ? "ArrowRight" : "ArrowLeft");
  }

  // ---- release overlay ----
  // The card grows out of the cover that was chosen (the front record, a
  // wall card) and shrinks back into it when closed: the card's own cover
  // is drawn first exactly over the small one, and the rest of the card
  // fades in around it. Without a visible cover (Random, Back/Forward) it
  // simply rises in and sinks out.
  const CARD_IN_MS = 340, CARD_OUT_MS = 260;
  const cache = new Map();
  let openedHere = false;
  let openedFrom = 0; // instance whose cover the card came from
  let closing = null;

  function visibleCover(inst) {
    for (const n of document.querySelectorAll('[data-i="' + inst + '"]')) {
      if (n.closest(".ghost") || dialog.contains(n)) continue;
      const r = n.getBoundingClientRect();
      if (r.width > 8 && r.bottom > 0 && r.right > 0 && r.top < innerHeight && r.left < innerWidth) return r;
    }
    return null;
  }

  // The keyframe that puts the card's cover over the rect `from`.
  function coverFrame(from) {
    const own = dialogBody.querySelector(".release-cover img, .release-cover .nocover");
    if (!from || !own) return null;
    const box = dialog.getBoundingClientRect();
    const c = own.getBoundingClientRect();
    if (!c.width) return null;
    const k = from.width / c.width;
    const x = from.left - box.left - k * (c.left - box.left);
    const y = from.top - box.top - k * (c.top - box.top);
    return { transformOrigin: "0 0", transform: `translate(${x}px, ${y}px) scale(${k})` };
  }

  function backdrop(keys, ms) {
    try {
      dialog.animate(keys, { duration: ms, easing: "linear", fill: "both", pseudoElement: "::backdrop" });
    } catch (e) {} // no ::backdrop animation here: it just appears
  }

  function animateIn(inst) {
    dialog.getAnimations({ subtree: true }).forEach((a) => a.cancel());
    if (reduceMotion.matches || !dialog.animate) return;
    const from = coverFrame(visibleCover(inst));
    if (from) {
      // The card's cover starts over the small one; the rest fades in fast.
      dialog.animate(
        [{ ...from, opacity: 0.4 }, { transformOrigin: "0 0", opacity: 1, offset: 0.35 }, { transformOrigin: "0 0", transform: "none", opacity: 1 }],
        { duration: CARD_IN_MS, easing: "cubic-bezier(.2,.7,.3,1)" }
      );
    } else {
      dialog.animate([{ opacity: 0, transform: "translateY(16px) scale(0.98)" }, { opacity: 1, transform: "none" }], { duration: 250, easing: "cubic-bezier(.2,.7,.2,1)" });
    }
    backdrop([{ opacity: 0 }, { opacity: 1 }], CARD_IN_MS);
  }

  // closeCard plays the way back and then really closes the dialog.
  function closeCard() {
    if (!dialog.open || closing) return;
    if (reduceMotion.matches || !dialog.animate) {
      dialog.close();
      return;
    }
    const to = coverFrame(visibleCover(openedFrom));
    const keys = to
      ? [{ transformOrigin: "0 0", transform: "none", opacity: 1 }, { transformOrigin: "0 0", opacity: 1, offset: 0.6 }, { ...to, opacity: 0 }]
      : [{ opacity: 1, transform: "none" }, { opacity: 0, transform: "translateY(16px) scale(0.98)" }];
    const anim = dialog.animate(keys, { duration: CARD_OUT_MS, easing: "cubic-bezier(.5,0,.75,.4)", fill: "forwards" });
    closing = anim;
    backdrop([{ opacity: 1 }, { opacity: 0 }], CARD_OUT_MS);
    anim.finished.then(
      () => {
        if (closing !== anim) return;
        closing = null;
        dialog.close();
        dialog.getAnimations({ subtree: true }).forEach((x) => x.cancel());
      },
      () => {} // cancelled: the card was reopened
    );
  }

  async function loadRelease(inst) {
    if (cache.has(inst)) return cache.get(inst);
    const res = await fetch("/release/" + inst + "?part=1", { headers: { Accept: "text/html" } });
    if (!res.ok) throw new Error(res.status);
    const doc = new DOMParser().parseFromString(await res.text(), "text/html");
    const art = doc.querySelector("article");
    if (!art) throw new Error("no article");
    cache.set(inst, art);
    return art;
  }

  async function openRelease(inst, fromHistory) {
    let art;
    try {
      art = await loadRelease(inst); // the card opens once it has something to show
    } catch (e) {
      location.href = "/release/" + inst;
      return;
    }
    if (closing) {
      const c = closing;
      closing = null; // reopened while closing: nothing closes it now
      dialog.getAnimations({ subtree: true }).forEach((x) => x.cancel());
      c.cancel();
    }
    dialogBody.replaceChildren(document.importNode(art, true));
    const wasOpen = dialog.open;
    if (!wasOpen) dialog.showModal();
    dialog.scrollTop = 0;
    openedFrom = inst;
    if (!wasOpen) animateIn(inst);
    document.title = art.dataset.title || baseTitle;
    dialog.setAttribute("aria-label", art.dataset.title || "");
    if (!fromHistory) {
      history.pushState({ rel: inst }, "", art.dataset.path || "/release/" + inst);
      openedHere = true;
    }
  }

  $("#overlay-close").addEventListener("click", closeCard);
  dialog.addEventListener("click", (e) => {
    if (e.target === dialog) closeCard(); // a click on the backdrop
  });
  dialog.addEventListener("cancel", (e) => {
    e.preventDefault(); // Esc: close with the animation
    closeCard();
  });
  dialog.addEventListener("close", () => {
    document.title = baseTitle;
    if (history.state && history.state.rel) {
      if (openedHere) history.back();
      else history.replaceState(null, "", listURL);
    }
    openedHere = false;
  });
  window.addEventListener("popstate", (e) => {
    const rel = e.state && e.state.rel;
    if (rel) openRelease(rel, true);
    else if (dialog.open) {
      openedHere = false;
      closeCard();
    }
  });

  // ---- toolbar ----
  const q = $("#q");
  const group = $("#group");
  q.value = state.q;
  group.value = state.group;
  let qTimer = 0;
  q.addEventListener("input", () => {
    clearTimeout(qTimer);
    qTimer = setTimeout(() => {
      state.q = q.value.trim();
      state.open = null;
      render();
    }, 150);
  });
  // After a toolbar control acts, the keyboard goes back to the stacks:
  // otherwise Enter would press the same button again instead of digging.
  const release = (el) => requestAnimationFrame(() => el.blur());
  group.addEventListener("change", () => {
    state.group = group.value;
    state.open = null;
    render();
    release(group);
  });
  const viewButtons = [...toolbar.querySelectorAll("[data-view]")];
  const setView = (v) => {
    state.view = v;
    viewButtons.forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.view === v)));
  };
  viewButtons.forEach((b) =>
    b.addEventListener("click", () => {
      setView(b.dataset.view);
      state.open = null;
      render();
      release(b);
    })
  );
  setView(state.view);
  const order = $("#order");
  const setOrder = (desc) => {
    state.desc = desc;
    order.setAttribute("aria-pressed", String(desc));
    order.querySelector("span").textContent = desc ? T.order_desc : T.order_asc;
    order.title = desc ? T.order_desc : T.order_asc;
    order.setAttribute("aria-label", order.title);
  };
  setOrder(state.desc);
  order.addEventListener("click", () => {
    // Keep the same stack in the middle; it now stands in the other order.
    const key = state.view === "stacks" && gs[center] ? gs[center].key : null;
    setOrder(!state.desc);
    state.open = null;
    render(key);
    release(order);
  });
  $("#random").addEventListener("click", () => {
    const list = filtered();
    if (list.length) openRelease(list[Math.floor(Math.random() * list.length)].i);
  });

  document.addEventListener("keydown", (e) => {
    if (dialog.open || e.altKey || e.metaKey || e.ctrlKey) return; // the dialog closes itself on Esc
    const t = e.target;
    if (t && (t.tagName === "INPUT" || t.tagName === "SELECT" || t.tagName === "TEXTAREA")) {
      if (e.key === "Escape" && t.tagName === "INPUT") t.blur();
      return;
    }
    if (!rail || center < 0) return;
    if (e.key === "Enter" && t && t.closest && t.closest("button, a")) return; // a focused control does its own thing
    let done;
    if (e.key === "Home") done = (go(0), true);
    else if (e.key === "End") done = (go(slots.length - 1), true);
    else done = press(e.key);
    if (done) e.preventDefault();
  });

  render();
  // Opened at /release/… with the list behind it (back from another site).
  if (history.state && history.state.rel) openRelease(history.state.rel, true);
})();
