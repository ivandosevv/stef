export const themeOptions = [
  { value: 'system', label: 'System' },
  { value: 'dark', label: 'Dark' },
  { value: 'light', label: 'Light' },
  { value: 'comfort', label: 'Eye comfort' },
] as const;

export type Theme = typeof themeOptions[number]['value'];
const storageKey = 'stef-theme';

// Appearance belongs to the view, so switching chats preserves the chosen palette.
export function readTheme(): Theme {
  try {
    const saved = localStorage.getItem(storageKey);
    return themeOptions.find(option => option.value === saved)?.value ?? 'dark';
  } catch { return 'dark'; }
}

export function applyTheme(theme: Theme): void {
  const resolved = theme === 'system' ? (matchMedia('(prefers-color-scheme: dark)').matches ? 'dark' : 'light') : theme;
  document.documentElement.dataset.theme = resolved;
  document.documentElement.dataset.themePreference = theme;
  const background = getComputedStyle(document.documentElement).getPropertyValue('--bg').trim();
  document.querySelector('meta[name="theme-color"]')?.setAttribute('content', background);
}

export function saveTheme(theme: Theme): void {
  try { localStorage.setItem(storageKey, theme); } catch { /* The current view still works without browser storage. */ }
  applyTheme(theme);
}
