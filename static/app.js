// The collection page. Records stand in stacks on a horizontal rail, like
// crates in a record shop:
//   ← →        move between stacks
//   ↓ Enter    in the overview: dig into the stack in the middle
//   ↓ ↑        while digging: flip through the records of that stack;
//              ↑ on the first record stops digging, like Esc
//   Enter      while digging: open the record at the front
//   Esc        close the record; then stop digging (back to the overview)
// Every vertical gesture is one of these keys, on every system: a wheel,
// two fingers on a trackpad or one finger on a screen moving DOWN is ↓,
// moving UP is ↑ (see press()). The server put the whole collection into
// <script id="data"> and the strings into <script id="texts">.
(() => {
  "use strict";
  const $ = (s, el = document) => el.querySelector(s);
  const data = JSON.parse($("#data").textContent || "[]");
  const T = JSON.parse($("#texts").textContent || "{}");
  const stage = $("#stage");
  const toolbar = $("#toolbar");
  const dialog = $("#overlay");
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
  const plural = (n) => {
    if (T.lang !== "ru") return n + " " + (n === 1 ? T.one : T.many);
    const m10 = n % 10, m100 = n % 100;
    if (m10 === 1 && m100 !== 11) return n + " " + T.one;
    if (m10 >= 2 && m10 <= 4 && (m100 < 12 || m100 > 14)) return n + " " + T.few;
    return n + " " + T.many;
  };
  const fold = (s) => s.toLowerCase().normalize("NFD").replace(/\p{M}/gu, "").replace(/ё/g, "е");
  const collator = new Intl.Collator(T.lang === "ru" ? "ru" : "en", { sensitivity: "base", numeric: true });
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
  // Artist (without "The"), then the original year, then the title.
  const byArtist = (a, b) => collator.compare(a.k, b.k) || (a.sy || 9999) - (b.sy || 9999) || collator.compare(a.t, b.t);

  function groups(list) {
    const m = new Map();
    const add = (key, label, rank, r) => {
      if (!m.has(key)) m.set(key, { key, label, rank, items: [] });
      m.get(key).items.push(r);
    };
    for (const r of list) {
      switch (state.group) {
        case "artist": {
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
            add(String(d), T.lang === "ru" ? d + "-е" : d + "s", 0, r);
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
    const qs = p.toString();
    const url = "/" + (qs ? "?" + qs : "");
    if (location.pathname === "/" && location.pathname + location.search !== url) {
      history.replaceState(history.state, "", url);
    }
    listURL = url;
    // The language switch comes back to this same view.
    document.querySelectorAll(".lang a").forEach((a) => {
      const u = new URL(a.href, location.origin);
      u.searchParams.set("back", url);
      a.href = u.pathname + u.search;
    });
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

  function cover(r, eager) {
    if (!r.c) return el("span", "nocover");
    const img = el("img");
    img.src = r.c;
    img.alt = "";
    img.loading = eager ? "eager" : "lazy";
    img.decoding = "async";
    img.width = img.height = 600;
    return img;
  }

  // Sleeve edges behind the front record: none for a single record.
  const edges = (behind) => (behind > 0 ? Math.min(behind, 40) * 1.5 + 2 + "px" : "0px");

  function renderRail(focusKey) {
    const crumb = el("div", "crumb");
    const back = el("button", "pill");
    back.type = "button";
    back.append(icon(LEFT), document.createTextNode(T.back));
    back.addEventListener("click", () => leaveStack());
    crumb.appendChild(back);
    crumb.hidden = !state.open;
    stage.appendChild(crumb);

    const wrap = el("div", "rail-wrap");
    rail = el("div", "rail" + (state.open ? " is-dig" : ""));
    prevBtn = button("nav-btn prev", T.prev, LEFT, () => go(center - 1));
    nextBtn = button("nav-btn next", T.next, RIGHT, () => go(center + 1));
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
    front.replaceChildren(cover(r, Math.abs(i - center) <= 3));
    paintFallen(pile, g, g.key === state.open ? idx : 0);
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
      label.append(el("span", "slot-title", g.label), el("span", "slot-count", plural(n)));
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

  // zoomTo switches between the overview and digging like a camera moving
  // in or out: the layout changes at once (bigger or smaller piles, stack i
  // in the middle), then the whole scene is drawn from where stack i was
  // before and glides to its new place and size.
  const ZOOM_MS = 320;
  const reduceMotion = window.matchMedia("(prefers-reduced-motion: reduce)");
  function zoomTo(i, change) {
    const pileOf = () => slots[i].querySelector(".pile").getBoundingClientRect();
    const wrap = rail.parentNode;
    const before = pileOf();
    change();
    go(i, true); // keep this stack in the middle of the resized rail
    const after = pileOf();
    if (reduceMotion.matches || !after.width || !wrap.animate) return;
    const box = wrap.getBoundingClientRect();
    const ax = after.left + after.width / 2, ay = after.top + after.height / 2;
    const bx = before.left + before.width / 2, by = before.top + before.height / 2;
    const origin = `${ax - box.left}px ${ay - box.top}px`;
    wrap.animate(
      [
        { transformOrigin: origin, transform: `translate(${bx - ax}px, ${by - ay}px) scale(${before.width / after.width})` },
        { transformOrigin: origin, transform: "none" },
      ],
      { duration: ZOOM_MS, easing: "cubic-bezier(.2,.7,.3,1)" }
    );
  }

  function enterStack(i) {
    const g = gs[i];
    if (!g) return;
    state.open = g.key;
    state.pos = 0;
    zoomTo(i, () => {
      rail.classList.add("is-dig");
      stage.querySelector(".crumb").hidden = false;
      paintSlot(i, 0);
    });
    syncURL();
  }

  function leaveStack() {
    if (!state.open) return;
    const i = gs.findIndex((g) => g.key === state.open);
    state.open = null;
    state.pos = 0;
    const out = () => {
      rail.classList.remove("is-dig");
      stage.querySelector(".crumb").hidden = true;
      if (i >= 0) paintSlot(i, 0);
    };
    if (i >= 0) zoomTo(i, out);
    else out();
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
  function go(i, instant) {
    if (!rail || !slots.length) return;
    i = Math.max(0, Math.min(slots.length - 1, i));
    const s = slots[i];
    const left = s.offsetLeft + s.offsetWidth / 2 - rail.clientWidth / 2;
    rail.scrollTo({ left, behavior: instant ? "instant" : "smooth" });
    if (instant) updateCenter();
  }

  let ticking = false;
  function onScroll() {
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
    if (best === center) return;
    const prev = center;
    if (slots[prev]) slots[prev].classList.remove("is-center");
    center = best;
    slots[center].classList.add("is-center");
    // Digging moves along with the middle stack; the one left behind is
    // squared up again with its first record in front.
    if (state.open && gs[center].key !== state.open) {
      const was = gs.findIndex((g) => g.key === state.open);
      state.open = gs[center].key;
      state.pos = 0;
      if (was >= 0) paintSlot(was, 0);
      paintSlot(center, 0);
    }
    // Covers near the middle load first.
    for (let i = Math.max(0, center - 3); i <= Math.min(slots.length - 1, center + 3); i++) {
      const img = slots[i].querySelector("img");
      if (img) img.loading = "eager";
    }
    prevBtn.disabled = center === 0;
    nextBtn.disabled = center === slots.length - 1;
    [...strip.children].forEach((b, i) => b.setAttribute("aria-current", i === center ? "true" : "false"));
    syncURL();
  }

  // press is the one place where keys and gestures turn into actions.
  function press(key) {
    if (!rail || center < 0) return false;
    const g = gs[center];
    const digging = state.open === g.key;
    switch (key) {
      case "ArrowRight":
        go(center + 1);
        break;
      case "ArrowLeft":
        go(center - 1);
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

  // Wheel and trackpad: up and down are the ↑ and ↓ keys (sideways scrolls
  // the rail natively).
  //
  // Direction: the gesture moves the way the hand does, like the ↓ key —
  // fingers or wheel down = the next record. Apple devices scroll
  // "naturally" by default, so a downward swipe arrives as a negative
  // deltaY there; elsewhere a wheel turned down gives a positive one. The
  // browser cannot tell whether natural scrolling is switched off.
  //
  // One step per gesture: a trackpad sends a stream of events for the swipe
  // and then keeps sending its inertia, whose deltas only shrink. After a
  // step the stream is ignored until one of these:
  //   - it stops (WHEEL_QUIET ms without events);
  //   - it changes direction;
  //   - a new swipe starts over the old one's inertia: the deltas fell to
  //     40% of the peak or less, and then two events in a row are at least
  //     2.5× the lowest one + 3 px and at least half the peak again.
  // WHEEL_MIN_GAP keeps one swipe from making two steps. Tuned on simulated
  // swipes with ±45% jitter and stalls: no double steps in 2000, while
  // swipes 220 ms apart, weak ones included, each make a step.
  const NATURAL = /Mac|iPhone|iPad|iPod/i.test((navigator.userAgentData && navigator.userAgentData.platform) || navigator.platform || "");
  const WHEEL_STEP = 30, WHEEL_QUIET = 150, WHEEL_MIN_GAP = 140;
  const wheel = { acc: 0, busy: false, timer: 0, at: 0, peak: 0, low: Infinity, sign: 0, hits: 0 };
  function wheelReset() {
    wheel.busy = false;
    wheel.acc = 0;
    wheel.peak = 0;
    wheel.low = Infinity;
    wheel.sign = 0;
    wheel.hits = 0;
  }
  function onWheel(e) {
    if (Math.abs(e.deltaX) > Math.abs(e.deltaY)) return; // native sideways scrolling
    e.preventDefault();
    const delta = e.deltaMode === 1 ? e.deltaY * 16 : e.deltaY; // lines → pixels
    if (!delta) return;
    const size = Math.abs(delta), sign = Math.sign(delta);
    const now = performance.now();
    clearTimeout(wheel.timer);
    wheel.timer = setTimeout(wheelReset, WHEEL_QUIET);
    if (wheel.busy) {
      // low is the smallest delta since the latest peak, so a swipe still
      // speeding up never counts as decayed.
      const turned = sign !== wheel.sign;
      const decayed = wheel.low <= wheel.peak * 0.4;
      const again = decayed && size >= wheel.low * 2.5 + 3 && size >= wheel.peak * 0.5;
      wheel.hits = again ? wheel.hits + 1 : 0;
      if (size > wheel.peak) {
        wheel.peak = size;
        wheel.low = Infinity;
      } else if (!again) {
        wheel.low = Math.min(wheel.low, size);
      }
      if (!(turned || wheel.hits >= 2) || now - wheel.at < WHEEL_MIN_GAP) return;
      wheelReset();
    }
    wheel.acc += delta;
    if (Math.abs(wheel.acc) < WHEEL_STEP) return;
    let dir = wheel.acc > 0 ? 1 : -1;
    if (NATURAL) dir = -dir;
    wheel.busy = true;
    wheel.acc = 0;
    wheel.at = now;
    wheel.sign = sign;
    wheel.peak = size;
    wheel.low = Infinity;
    wheel.hits = 0;
    press(dir > 0 ? "ArrowDown" : "ArrowUp");
  }

  // Touch: sideways swipes scroll the rail natively; a vertical swipe is a
  // key press (finger down = ↓).
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
    // A finger moving down is ↓, up is ↑ (sideways the rail scrolls natively).
    if (Math.abs(dy) > 40 && Math.abs(dy) > Math.abs(dx) * 1.5) press(dy > 0 ? "ArrowDown" : "ArrowUp");
  }

  // ---- release overlay ----
  let openedHere = false;
  async function openRelease(inst, fromHistory) {
    dialogBody.textContent = "";
    if (!dialog.open) dialog.showModal();
    dialog.scrollTop = 0;
    let html = "";
    try {
      const res = await fetch("/release/" + inst + "?part=1", { headers: { Accept: "text/html" } });
      if (!res.ok) throw new Error(res.status);
      html = await res.text();
    } catch (e) {
      location.href = "/release/" + inst;
      return;
    }
    const doc = new DOMParser().parseFromString(html, "text/html");
    const art = doc.querySelector("article");
    if (!art) return;
    dialogBody.replaceChildren(document.importNode(art, true));
    document.title = art.dataset.title || baseTitle;
    dialog.setAttribute("aria-label", art.dataset.title || "");
    if (!fromHistory) {
      history.pushState({ rel: inst }, "", art.dataset.path || "/release/" + inst);
      openedHere = true;
    }
  }

  $("#overlay-close").addEventListener("click", () => dialog.close());
  dialog.addEventListener("click", (e) => {
    if (e.target === dialog) dialog.close(); // a click on the backdrop
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
      dialog.close();
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
