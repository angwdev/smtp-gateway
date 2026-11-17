(function () {
  var dark = false;
  try {
    var mode = localStorage.getItem('mui-mode') || 'system';
    dark = mode === 'dark' || (mode === 'system' && matchMedia('(prefers-color-scheme: dark)').matches);
  } catch {
    /* storage blocked: stay light */
  }
  document.documentElement.classList.add(dark ? 'dark' : 'light');
})();
