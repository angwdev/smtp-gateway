// Progressive enhancement only: every action works without this file.

// Copy buttons: data-copy="<element id>" copies that element's text.
document.addEventListener("click", function (e) {
  var btn = e.target.closest("[data-copy]");
  if (!btn) return;
  var el = document.getElementById(btn.dataset.copy);
  if (!el || !navigator.clipboard) return;
  navigator.clipboard.writeText(el.textContent.trim()).then(function () {
    var label = btn.querySelector(".label");
    var live = document.getElementById("live");
    btn.classList.add("copied");
    if (label) { label.dataset.text = label.dataset.text || label.textContent; label.textContent = "Copied"; }
    if (live) live.textContent = "Copied to clipboard";
    setTimeout(function () {
      btn.classList.remove("copied");
      if (label) label.textContent = label.dataset.text;
    }, 1600);
  });
});

// Busy state for slow submits (upstream test, create): disable and relabel.
document.addEventListener("submit", function (e) {
  var form = e.target;
  var btn = e.submitter || form.querySelector("button[type=submit], button:not([type])");
  if (!btn || !btn.dataset.busy) return;
  if (btn.getAttribute("aria-busy") === "true") { e.preventDefault(); return; }
  setTimeout(function () {
    btn.setAttribute("aria-busy", "true");
    var label = btn.querySelector(".label") || btn;
    label.textContent = btn.dataset.busy;
  }, 0);
});

// Dismissable notices.
document.addEventListener("click", function (e) {
  var d = e.target.closest(".dismiss");
  if (d) d.closest(".notice").remove();
});

// Theme picker.
(function () {
  var sel = document.getElementById("theme");
  if (!sel) return;
  sel.value = document.documentElement.getAttribute("data-theme") || "dark";
  sel.closest(".theme").hidden = false;
  sel.addEventListener("change", function () {
    document.documentElement.setAttribute("data-theme", sel.value);
    try { localStorage.setItem("theme", sel.value); } catch (e) {}
  });
})();
