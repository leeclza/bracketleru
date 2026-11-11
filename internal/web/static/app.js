// Gambar garis penghubung antar match (kayak Liquipedia) + live preview di halaman depan.
(function () {
  const NS = "http://www.w3.org/2000/svg";

  function drawWires(root) {
    root.querySelectorAll(".board").forEach((board) => {
      const svg = board.querySelector("svg.wires");
      if (!svg) return;
      svg.innerHTML = "";
      const box = board.getBoundingClientRect();
      svg.setAttribute("width", board.scrollWidth);
      svg.setAttribute("height", board.scrollHeight);

      board.querySelectorAll(".match[data-feeds]").forEach((target) => {
        target.dataset.feeds.split(" ").forEach((f) => {
          const id = f.slice(0, -1), sideB = f.endsWith("b");
          const src = board.querySelector('.match[data-id="' + id + '"]');
          if (!src) return;
          const s = src.querySelector("summary").getBoundingClientRect();
          const row = target.querySelector(sideB ? ".sb" : ".sa").getBoundingClientRect();
          const x1 = s.right - box.left, y1 = s.top + s.height / 2 - box.top;
          const x2 = row.left - box.left, y2 = row.top + row.height / 2 - box.top;
          const mx = Math.max(x1 + 10, x2 - 16);
          const p = document.createElementNS(NS, "path");
          p.setAttribute("d", `M${x1} ${y1} H${mx} V${y2} H${x2}`);
          if (src.classList.contains("done")) p.setAttribute("class", "on");
          svg.appendChild(p);
        });
      });
    });
  }

  let raf;
  const redraw = () => { cancelAnimationFrame(raf); raf = requestAnimationFrame(() => drawWires(document)); };
  window.addEventListener("resize", redraw);
  window.addEventListener("load", redraw);
  document.addEventListener("DOMContentLoaded", () => {
    redraw();
    if (document.fonts) document.fonts.ready.then(redraw);

    // tutup popup skor lain kalau buka satu
    document.addEventListener("toggle", (e) => {
      const d = e.target;
      if (d.classList && d.classList.contains("match") && d.open) {
        document.querySelectorAll(".match[open]").forEach((o) => o !== d && (o.open = false));
      }
    }, true);

    // live preview di halaman bikin turnamen
    const form = document.getElementById("create-form");
    const out = document.querySelector("[data-preview]");
    if (!form || !out) return;
    let timer, seq = 0;
    const update = () => {
      clearTimeout(timer);
      timer = setTimeout(async () => {
        const fd = new FormData(form);
        if (!String(fd.get("participants") || "").trim()) {
          out.innerHTML = '<div class="empty">Isi nama peserta di sidebar kiri, bracket-nya langsung keliatan di sini 👀</div>';
          return;
        }
        const my = ++seq;
        try {
          const res = await fetch("/preview", { method: "POST", body: new URLSearchParams(fd) });
          const html = await res.text();
          if (my !== seq) return;
          out.innerHTML = html;
          redraw();
        } catch (_) { /* biarin, coba lagi pas ngetik berikutnya */ }
      }, 250);
    };
    form.addEventListener("input", update);
    form.addEventListener("change", update);
    update();
  });
})();
