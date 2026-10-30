export type Design = 'minimal' | 'interactive';
const storageKey = 'stef-design';

export function readDesign(): Design {
  try { return localStorage.getItem(storageKey) === 'interactive' ? 'interactive' : 'minimal'; }
  catch { return 'minimal'; }
}

// Visual presentation is local; changing it never requests agent output.
export function saveDesign(design: Design): void {
  try { localStorage.setItem(storageKey, design); } catch { /* The view works without storage. */ }
}
