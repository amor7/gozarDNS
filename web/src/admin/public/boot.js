// Runs before first paint so the page never flashes the wrong theme or direction.
try {
  var t = localStorage.getItem("gz-theme");
  if (t === "dark" || ((!t || t === "system") && matchMedia("(prefers-color-scheme: dark)").matches)) document.documentElement.classList.add("dark");
  if (localStorage.getItem("gz-lang") === "en") { document.documentElement.lang = "en"; document.documentElement.dir = "ltr"; }
} catch (e) {}
