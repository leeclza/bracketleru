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

    // popup skor: cuma satu yang kebuka, fokus ke kolom skor, bisa ditutup
    // lewat tombol ✕, klik di luar dialog, atau tombol Esc.
    const closeAll = () => document.querySelectorAll(".match[open]").forEach((o) => (o.open = false));
    document.addEventListener("toggle", (e) => {
      const d = e.target;
      if (d.classList && d.classList.contains("match") && d.open) {
        document.querySelectorAll(".match[open]").forEach((o) => o !== d && (o.open = false));
        const inp = d.querySelector(".score-pop input[type=number]");
        if (inp) setTimeout(() => inp.focus({ preventScroll: true }), 30);
      }
    }, true);
    document.addEventListener("click", (e) => {
      if (e.target.closest("[data-close]")) { closeAll(); return; }
      const open = document.querySelector(".match[open]");
      // klik di area gelap (di luar dialog & kartunya) = tutup
      if (open && !e.target.closest(".score-pop") && !open.querySelector("summary").contains(e.target)) closeAll();
    });
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeAll(); });

    // ---- countdown hapus otomatis (data-expires = unix ms) ----
    const fmtLeft = (ms) => {
      if (ms <= 0) return "sebentar lagi";
      const s = Math.floor(ms / 1000);
      const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600), m = Math.floor((s % 3600) / 60), sec = s % 60;
      const p = [];
      if (d) p.push(d + " hari");
      if (d || h) p.push(h + " jam");
      p.push(m + " menit", sec + " detik");
      return p.join(" ");
    };
    const tick = () => document.querySelectorAll("[data-expires]").forEach((el) => {
      const prefix = el.dataset.prefix ?? "akan dihapus dalam ";
      el.textContent = prefix + fmtLeft(Number(el.dataset.expires) - Date.now());
    });
    tick();
    setInterval(tick, 1000);

    // ---- halaman turnamen: simpan skor & aksi admin tanpa reload ----
    // Form POST dikirim lewat fetch. fetch otomatis ngikutin redirect ke
    // halaman turnamen, lalu cuma isi <main> yang diganti (scroll tetap).
    const main = document.querySelector("main.workspace");
    const onTournament = /^\/t\/[^/]+$/.test(location.pathname);

    async function swapFrom(res) {
      const html = await res.text();
      const doc = new DOMParser().parseFromString(html, "text/html");
      const fresh = doc.querySelector("main.workspace");
      const samePage = new URL(res.url).pathname === location.pathname;
      if (!fresh || !samePage) { location.href = res.url; return; }

      // simpan posisi scroll semua area yang bisa di-scroll
      const scrollers = [...main.querySelectorAll(".stage, .sidebar, .board-scroll")]
        .map((el) => [el.scrollTop, el.scrollLeft]);
      const openDetails = [...main.querySelectorAll("details.side-section")].map((d) => d.open);

      main.innerHTML = fresh.innerHTML;

      main.querySelectorAll(".stage, .sidebar, .board-scroll").forEach((el, i) => {
        if (scrollers[i]) [el.scrollTop, el.scrollLeft] = scrollers[i];
      });
      main.querySelectorAll("details.side-section").forEach((d, i) => {
        if (openDetails[i] !== undefined) d.open = openDetails[i];
      });
      document.title = doc.title;
      history.replaceState(null, "", location.pathname); // buang ?err= dari URL
      redraw();
    }

    async function send(url, opts, busyEl) {
      if (busyEl) busyEl.disabled = true;
      main.classList.add("busy");
      try {
        const res = await fetch(url, Object.assign({ credentials: "same-origin" }, opts));
        await swapFrom(res);
      } catch (_) {
        alert("Gagal terhubung ke server, coba lagi.");
        if (busyEl) busyEl.disabled = false;
      } finally {
        main.classList.remove("busy");
      }
    }

    if (main && onTournament) {
      main.addEventListener("submit", (e) => {
        const f = e.target;
        if ((f.method || "").toLowerCase() !== "post" || /\/delete$/.test(f.action)) return; // hapus = pindah halaman biasa
        e.preventDefault();
        send(f.action, { method: "POST", body: new URLSearchParams(new FormData(f)) }, f.querySelector("button"));
      });
      main.addEventListener("click", (e) => {
        const b = e.target.closest("[data-reload]");
        if (b) send(location.pathname, {}, b);
      });
    }

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
