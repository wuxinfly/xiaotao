export type Theme = 'dark' | 'light';
const themeKey = 'xiaotao.theme';

export function readTheme(): Theme {
  try { return localStorage.getItem(themeKey) === 'light' ? 'light' : 'dark'; }
  catch { return 'dark'; }
}

export function applyTheme(theme: Theme) {
  document.documentElement.dataset.theme = theme;
  document.documentElement.style.colorScheme = theme;
}

export function saveTheme(theme: Theme) {
  applyTheme(theme);
  try { localStorage.setItem(themeKey, theme); } catch { /* Keep switching available without storage. */ }
}

applyTheme(readTheme());
// The standalone HUD shares the origin, but has its own React root.
window.addEventListener('storage', event => {
  if (event.key === themeKey || event.key === null) {
    applyTheme(readTheme());
    window.dispatchEvent(new Event('xiaotao:theme'));
  }
});
