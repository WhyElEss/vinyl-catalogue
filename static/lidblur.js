// Blurs the cover of a crate lid off the page's main thread (app.js,
// blurCover): three box blurs — the usual stand-in for a Gaussian — over
// premultiplied RGBA, along rows and then columns, with everything outside
// the picture counted as clear (as an SVG blur does). In the owner's Safari
// this arithmetic took 14–20 ms a lid on the main thread (2026-11-11).
"use strict";

function boxPass(s, t, W, r, step, line) {
  const k = 1 / (2 * r + 1), st = step * 4;
  for (let l = 0; l < W; l++) {
    const o = l * line * 4;
    let a0 = 0, a1 = 0, a2 = 0, a3 = 0;
    for (let x = 0; x < r; x++) {
      const p = o + x * st;
      a0 += s[p]; a1 += s[p + 1]; a2 += s[p + 2]; a3 += s[p + 3];
    }
    for (let x = 0; x < W; x++) {
      if (x + r < W) {
        const p = o + (x + r) * st;
        a0 += s[p]; a1 += s[p + 1]; a2 += s[p + 2]; a3 += s[p + 3];
      }
      const q = o + x * st;
      t[q] = a0 * k; t[q + 1] = a1 * k; t[q + 2] = a2 * k; t[q + 3] = a3 * k;
      if (x - r >= 0) {
        const p = o + (x - r) * st;
        a0 -= s[p]; a1 -= s[p + 1]; a2 -= s[p + 2]; a3 -= s[p + 3];
      }
    }
  }
}

onmessage = (e) => {
  const { id, W, boxes, buf } = e.data;
  const t0 = performance.now();
  const d = new Uint8ClampedArray(buf), n4 = W * W * 4;
  const A = new Float32Array(n4), B = new Float32Array(n4);
  for (let i = 0; i < n4; i += 4) {
    const a = d[i + 3] / 255;
    A[i] = d[i] * a; A[i + 1] = d[i + 1] * a; A[i + 2] = d[i + 2] * a; A[i + 3] = a;
  }
  for (const box of boxes) {
    const r = (box - 1) / 2;
    boxPass(A, B, W, r, 1, W); // rows
    boxPass(B, A, W, r, W, 1); // columns
  }
  for (let i = 0; i < n4; i += 4) {
    const a = A[i + 3];
    d[i + 3] = a * 255;
    d[i] = a > 0 ? A[i] / a : 0;
    d[i + 1] = a > 0 ? A[i + 1] / a : 0;
    d[i + 2] = a > 0 ? A[i + 2] / a : 0;
  }
  postMessage({ id, buf, ms: performance.now() - t0 }, [buf]);
};
