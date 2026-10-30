import { describe, expect, it } from 'vitest';
import { effectValue, lineSuggestion, styleEffects, visibleLines } from './interaction';
import { diffHunks } from './diff';

describe('local change exploration', () => {
  it('keeps only the visible side’s EOF marker without modifying source', () => {
    const lines = diffHunks('@@ -1 +1 @@\n-old\n\\ No newline at end of file\n+\tnew  \n\\ No newline at end of file\n')[0];
    expect(visibleLines(lines, 'addition').map(line => line.text)).toEqual(['@@ -1 +1 @@', '\tnew  ', '\\ No newline at end of file']);
    expect(visibleLines(lines, 'removal').map(line => line.text)).toEqual(['@@ -1 +1 @@', 'old', '\\ No newline at end of file']);
  });
  it('targets the exact saved revision and bounds long unicode notes without splitting characters', () => {
    const line = { kind: 'addition' as const, prefix: '+', text: '🙂'.repeat(1000), newLine: 19 };
    const draft = lineSuggestion('src/main.ts', '18446744073709551615', line);
    expect(draft).toContain('revision 18446744073709551615, added line 19');
    expect(draft).toContain('+🙂');
    expect(draft.endsWith('…\nSuggestion: ')).toBe(true);
    expect(draft.length).toBeLessThanOrEqual(500);
    expect(draft).not.toMatch(/[\uD800-\uDBFF](?![\uDC00-\uDFFF])/u);
  });
});

describe('saved literal style effect samples', () => {
  const patch = '@@ -1 +1 @@\n-.card { padding: 4px; color: #000; --bg: #fff; }\n+.card { padding: 20px; color: #fff; --bg: #000; }\n';
  it('interpolates exact changed literal properties independently', () => {
    const effects = styleEffects(patch, 'src/card.css');
    expect(effects.map(effect => effect.sourceProperty)).toEqual(['padding', 'color', '--bg']);
    expect(effectValue(effects[0], 0)).toBe('4px');
    expect(effectValue(effects[0], 50)).toBe('12px');
    expect(effectValue(effects[0], 100)).toBe('20px');
    expect(effectValue(effects[1], 50)).toBe('rgba(128, 128, 128, 1)');
    expect(effects[2].property).toBe('background-color');
  });
  it('never samples executable source, unsupported CSS, ambiguous declarations, or unsafe values', () => {
    expect(styleEffects(patch, 'main.ts')).toEqual([]);
    expect(styleEffects('@@ -1 +1 @@\n-.x { --Brand: #000; }\n+.x { --brand: #fff; }\n', 'style.css')).toEqual([]);
    expect(styleEffects('@@ -1 +1 @@\n-.x { --Brand: #000; }\n+.x { --Brand: #fff; }\n', 'style.css')[0].sourceProperty).toBe('--Brand');
    for (const [before, after] of [['red', 'url(https://example.test/pixel)'], ['4px', 'expression(alert(1))'], ['4px', '9000px'], ['4px', 'var(--unknown)']]) {
      expect(styleEffects('@@ -1 +1 @@\n-.x { padding: ' + before + '; }\n+.x { padding: ' + after + '; }\n', 'style.css')).toEqual([]);
    }
    expect(styleEffects('@@ -1 +1 @@\n-.x { background-color: #fff; }\n+.x { background-color: url(https://example.test/pixel); }\n', 'style.css')).toEqual([]);
    const ambiguous = '@@ -1,2 +1,2 @@\n-.a { color: #000; }\n-.b { color: #111; }\n+.a { color: #fff; }\n+.b { color: #eee; }\n';
    expect(styleEffects(ambiguous, 'style.css')).toEqual([]);
    expect(styleEffects('@@ -1 +1 @@\n-.x { position: absolute; }\n+.x { position: fixed; }\n', 'style.css')).toEqual([]);
  });
  it('accepts bounded opacity, alpha colors, and zero-to-size samples while clamping controls', () => {
    const effects = styleEffects('@@ -1 +1 @@\n-.x { opacity: .25; border-radius: 0; background-color: #0000; }\n+.x { opacity: 1; border-radius: 8px; background-color: rgba(255, 0, 0, 1); }\n', 'style.css');
    expect(effects).toHaveLength(3);
    expect(effectValue(effects[0], -30)).toBe('0.25');
    expect(effectValue(effects[1], 40)).toBe('3.2px');
    expect(effectValue(effects[2], 130)).toBe('rgba(255, 0, 0, 1)');
  });
});
