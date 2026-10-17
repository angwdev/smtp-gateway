// Runs before first paint (not deferred) so the stored theme never flashes.
// Default is dark; "system" follows the OS.
(function () {
  var t = "dark";
  try { t = localStorage.getItem("theme") || "dark"; } catch (e) {}
  document.documentElement.setAttribute("data-theme", t);
})();
