// Runs before first paint (loaded synchronously in <head>) so a saved choice
// never flashes the wrong theme. Kept as its own file because the CSP forbids inline scripts.
try {
  const saved = localStorage.getItem("cleanmywt-theme");
  if (saved === "light" || saved === "dark") document.documentElement.dataset.theme = saved;
} catch { /* storage blocked: fall back to the OS setting */ }
