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
      // klik di mana pun di luar dialog = cuma nutup, gak ngebuka kartu lain
      if (open && !e.target.closest(".score-pop")) {
        e.preventDefault();
        e.stopPropagation();
        closeAll();
      }
    }, true);
    document.addEventListener("keydown", (e) => { if (e.key === "Escape") closeAll(); });

    // dropdown akun: tutup kalau klik di luar / tekan Esc
    document.addEventListener("click", (e) => {
      document.querySelectorAll(".user-menu[open]").forEach((m) => { if (!m.contains(e.target)) m.open = false; });
    });
    document.addEventListener("keydown", (e) => {
      if (e.key === "Escape") document.querySelectorAll(".user-menu[open]").forEach((m) => (m.open = false));
    });

    // ---- countdown hapus otomatis (data-expires = unix ms) ----
    const fmtLeft = (ms) => {
      if (ms <= 0) return "sebentar lagi";
      const s = Math.floor(ms / 1000);
      const d = Math.floor(s / 86400), h = Math.floor((s % 86400) / 3600);
      if (!d && !h) return "kurang dari 1 jam";
      const p = [];
      if (d) p.push(d + " hari");
      if (h) p.push(h + " jam");
      return p.join(" ");
    };
    const tick = () => document.querySelectorAll("[data-expires]").forEach((el) => {
      const prefix = el.dataset.prefix ?? "akan dihapus dalam ";
      el.textContent = prefix + fmtLeft(Number(el.dataset.expires) - Date.now());
    });
    tick();
    setInterval(tick, 60000);

    // ---- toast pojok kanan bawah: hijau = berhasil, merah = gagal ----
    function toast(msg, ok) {
      let box = document.querySelector(".toasts");
      if (!box) { box = document.createElement("div"); box.className = "toasts"; document.body.appendChild(box); }
      const t = document.createElement("div");
      t.className = "toast " + (ok ? "ok" : "fail");
      t.setAttribute("role", ok ? "status" : "alert");
      t.textContent = (ok ? "✓ " : "✕ ") + msg;
      box.appendChild(t);
      setTimeout(() => { t.classList.add("hide"); setTimeout(() => t.remove(), 300); }, 3000);
    }
    // toast titipan dari halaman sebelumnya (misal habis hapus turnamen)
    try {
      const pending = sessionStorage.getItem("toast");
      if (pending) { sessionStorage.removeItem("toast"); toast(pending, true); }
    } catch (_) {}

    // ---- konfirmasi "yakin?" buat aksi berbahaya (form[data-confirm]) ----
    document.addEventListener("submit", (e) => {
      const f = e.target;
      if (!f.dataset || !f.dataset.confirm || f.dataset.confirmed) return;
      e.preventDefault();
      e.stopImmediatePropagation();
      const wrap = document.createElement("div");
      wrap.className = "confirm-backdrop";
      wrap.innerHTML = '<div class="confirm-card" role="alertdialog" aria-modal="true">' +
        '<p class="confirm-title">Anda yakin?</p><p class="confirm-text"></p>' +
        '<div class="confirm-actions"><button type="button" class="btn" data-no>Batal</button>' +
        '<button type="button" class="btn danger" data-yes></button></div></div>';
      wrap.querySelector(".confirm-text").textContent = f.dataset.confirm;
      wrap.querySelector("[data-yes]").textContent = f.dataset.confirmBtn || "Ya, lanjut";
      const close = () => { wrap.remove(); document.removeEventListener("keydown", onKey); };
      const onKey = (ev) => { if (ev.key === "Escape") close(); };
      wrap.addEventListener("click", (ev) => {
        if (ev.target === wrap || ev.target.closest("[data-no]")) close();
        else if (ev.target.closest("[data-yes]")) {
          close();
          if (/\/delete$/.test(f.action)) { try { sessionStorage.setItem("toast", f.dataset.ok || "Berhasil"); } catch (_) {} }
          f.dataset.confirmed = "1";
          f.requestSubmit();
          delete f.dataset.confirmed;
        }
      });
      document.addEventListener("keydown", onKey);
      document.body.appendChild(wrap);
      wrap.querySelector("[data-no]").focus();
    }, true);

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

    async function send(url, opts, busyEl, okMsg) {
      if (busyEl) busyEl.disabled = true;
      main.classList.add("busy");
      try {
        const res = await fetch(url, Object.assign({ credentials: "same-origin" }, opts));
        const err = new URL(res.url).searchParams.get("err");
        await swapFrom(res);
        if (err) {
          main.querySelectorAll(".alert").forEach((a) => a.remove()); // pesan error cukup di toast
          toast(err, false);
        } else if (okMsg) toast(okMsg, true);
      } catch (_) {
        toast("Gagal terhubung ke server, coba lagi.", false);
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
        const ok = f.dataset.ok || (f.classList.contains("score-form") ? "Skor disimpan" : "Perubahan disimpan");
        send(f.action, { method: "POST", body: new URLSearchParams(new FormData(f)) }, f.querySelector("button"), ok);
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
