import { diffHunks, type DiffLine } from './diff';

export type DiffFilter = 'all' | 'changes' | 'addition' | 'removal';
export const lineKey = (line: DiffLine): string => [line.kind, line.oldLine ?? '', line.newLine ?? ''].join(':');
export const selectableLine = (line: DiffLine): boolean => ['addition', 'removal', 'context'].includes(line.kind);
export function lineLabel(line: DiffLine): string {
  return line.kind === 'addition' ? 'Added line ' + line.newLine : line.kind === 'removal' ? 'Removed line ' + line.oldLine : 'Context line ' + line.newLine;
}

// Retain only newline markers belonging to a visible source row when filtering.
export function visibleLines(lines: DiffLine[], filter: DiffFilter): DiffLine[] {
  if (filter === 'all') return lines;
  let previousVisible = false;
  return lines.filter(line => {
    if (selectableLine(line)) {
      previousVisible = filter === 'changes' ? line.kind !== 'context' : line.kind === filter;
      return previousVisible;
    }
    return line.kind === 'meta' && line.text === '\\ No newline at end of file' ? previousVisible : true;
  });
}

function shortened(text: string, budget: number): string {
  if (text.length <= budget) return text;
  let result = '';
  for (const character of text) {
    if (result.length + character.length + 1 > budget) break;
    result += character;
  }
  return result + '…';
}

// Include the revision, so the developer's note never silently targets a later save.
export function lineSuggestion(path: string, revision: string, line: DiffLine): string {
  const heading = 'About ' + shortened(path, 160) + ', revision ' + revision + ', ' + lineLabel(line).toLowerCase() + ':\n';
  const ending = '\nSuggestion: ';
  const source = line.prefix + line.text;
  const budget = Math.max(0, 500 - heading.length - ending.length);
  return heading + shortened(source, budget) + ending;
}

type Color = [number, number, number, number];
const namedColors: Record<string, Color> = { black: [0, 0, 0, 1], white: [255, 255, 255, 1], red: [255, 0, 0, 1], green: [0, 128, 0, 1], blue: [0, 0, 255, 1], transparent: [0, 0, 0, 0] };
function literalColor(value: string): Color | undefined {
  const lower = value.toLowerCase();
  if (Object.hasOwn(namedColors, lower)) return namedColors[lower];
  const hex = /^#([\da-f]{3,4}|[\da-f]{6}|[\da-f]{8})$/i.exec(value)?.[1];
  if (hex) {
    const expanded = hex.length <= 4 ? [...hex].map(character => character + character).join('') : hex;
    return [parseInt(expanded.slice(0, 2), 16), parseInt(expanded.slice(2, 4), 16), parseInt(expanded.slice(4, 6), 16), expanded.length === 8 ? parseInt(expanded.slice(6, 8), 16) / 255 : 1];
  }
  const rgb = /^rgba?\(\s*(\d{1,3})\s*,\s*(\d{1,3})\s*,\s*(\d{1,3})(?:\s*,\s*(0|1|0?\.\d+))?\s*\)$/i.exec(value);
  if (rgb && rgb.slice(1, 4).every(number => Number(number) <= 255)) return [Number(rgb[1]), Number(rgb[2]), Number(rgb[3]), rgb[4] ? Number(rgb[4]) : 1];
  return undefined;
}

export interface StyleEffect {
  selector?: string;
  sourceProperty: string;
  property: string;
  before: string;
  after: string;
  color?: { before: Color; after: Color };
  length?: { before: number; after: number; unit: string };
}
const sizeLimits: Record<string, number> = { padding: 48, gap: 48, 'border-radius': 64, 'border-width': 12, 'font-size': 48, width: 240, height: 120 };
const colorProperties = new Set(['color', 'background', 'background-color', 'border-color']);
function safeEffect(property: string, before: string, after: string): StyleEffect | undefined {
  if (before === after) return undefined;
  if (colorProperties.has(property) || property.startsWith('--')) {
    const oldColor = literalColor(before), newColor = literalColor(after);
    if (oldColor && newColor) return { sourceProperty: property, property: property === 'background' || property.startsWith('--') ? 'background-color' : property, before, after, color: { before: oldColor, after: newColor } };
  }
  if (property === 'opacity') {
    if (/^(?:0|1|0?\.\d+)$/.test(before) && /^(?:0|1|0?\.\d+)$/.test(after)) return { sourceProperty: property, property, before, after, length: { before: Number(before), after: Number(after), unit: '' } };
  }
  const limit = Object.hasOwn(sizeLimits, property) ? sizeLimits[property] : undefined;
  if (!limit) return undefined;
  const parse = (value: string) => /^(0|\d+(?:\.\d+)?)(px|rem|em)?$/.exec(value);
  const oldSize = parse(before), newSize = parse(after);
  if (!oldSize || !newSize) return undefined;
  const unit = oldSize[2] ?? newSize[2] ?? 'px';
  if ((oldSize[2] && oldSize[2] !== unit) || (newSize[2] && newSize[2] !== unit) || (!oldSize[2] && Number(oldSize[1]) !== 0) || (!newSize[2] && Number(newSize[1]) !== 0)) return undefined;
  const bound = unit === 'px' ? limit : Math.min(limit / 16, 8);
  if (Number(oldSize[1]) > bound || Number(newSize[1]) > bound) return undefined;
  return { sourceProperty: property, property, before, after, length: { before: Number(oldSize[1]), after: Number(newSize[1]), unit } };
}

interface Declaration { selector: string; property: string; values: string[]; }

function capturedStyles(source: string, path: string): string | undefined {
  if (/\.(?:css|scss|less)$/i.test(path)) return source;
  // Markup needs complete captured style tags. An interior snippet could be a
  // script string or comment; absence of context is not evidence of CSS scope.
  if (/<script\b|<!--|-->/i.test(source)) return undefined;
  const sections = [...source.matchAll(/^\s*<style(?:\s+(?:global|lang=["'](?:css|scss|less)["']))*\s*>\s*$([\s\S]*?)^\s*<\/style>\s*$/gm)];
  if (sections.length !== 1) return undefined;
  const section = sections[0];
  if (source.slice(0, section.index).trim() || source.slice(section.index! + section[0].length).trim()) return undefined;
  return section[1];
}

function declarations(lines: DiffLine[], kind: 'addition' | 'removal', path: string): Map<string, Declaration> {
  const source = capturedStyles(lines.filter(line => line.kind === kind || line.kind === 'context').map(line => line.text).join('\n'), path);
  const values = new Map<string, Declaration>();
  if (source === undefined) return values;
  const clean = source.replace(/\/\*[\s\S]*?\*\//g, ' ');
  if (clean.includes('/*') || clean.includes('*/')) return values;
  let start = 0, depth = 0, opening = 0, selector = '', nested = false;
  for (let index = 0; index < clean.length; index++) {
    if (clean[index] === '{') {
      if (depth === 0) { selector = clean.slice(start, index).trim(); opening = index; nested = false; }
      else nested = true;
      depth++;
    } else if (clean[index] === '}') {
      if (depth > 0 && --depth === 0 && !nested && selector && /^[.#:[\]\w\s>,+~*()=-]+$/.test(selector)) {
        // Complete flat rules only. At-rule/nesting scope, quoted selectors,
        // partial declarations and source strings are intentionally unsupported.
        const scope = selector.replace(/\s+/g, ' ');
        const body = clean.slice(opening + 1, index);
        for (const declaration of body.split(';')) {
          const match = /^\s*([\w-]+)\s*:\s*([^;{}]+?)\s*$/.exec(declaration);
          if (!match) continue;
          const property = match[1].startsWith('--') ? match[1] : match[1].toLowerCase();
          const key = scope + '\n' + property;
          const existing = values.get(key);
          values.set(key, { selector: scope, property, values: [...(existing?.values ?? []), match[2].trim()] });
        }
      }
      if (depth === 0) start = index + 1;
    }
  }
  return values;
}

// Each sample represents one unambiguous paired declaration, not a reconstruction of the app.
export function styleEffects(diff: string, path: string): StyleEffect[] {
  if (!/\.(?:css|scss|less|svelte|html)$/i.test(path)) return [];
  const effects: StyleEffect[] = [];
  for (const hunk of diffHunks(diff)) {
    const before = declarations(hunk, 'removal', path), after = declarations(hunk, 'addition', path);
    for (const [key, declaration] of before) {
      const next = after.get(key);
      if (declaration.values.length !== 1 || next?.values.length !== 1) continue;
      const effect = safeEffect(declaration.property, declaration.values[0], next.values[0]);
      if (effect) effects.push({ ...effect, selector: declaration.selector });
    }
  }
  return effects.slice(0, 12);
}

export interface SavedComparison {
  prefix: string;
  before: string;
  after: string;
  suffix: string;
  beforeLines: number[];
  afterLines: number[];
  beforeNoNewline: boolean;
  afterNoNewline: boolean;
}

// Compare the whole contiguous saved replacement; never guess which lines in a
// multi-line rewrite correspond. Only a shared prefix/suffix is inferred locally.
export function savedComparison(lines: DiffLine[], selected: DiffLine): SavedComparison | undefined {
  if (selected.kind !== 'addition' && selected.kind !== 'removal') return undefined;
  const selectedIndex = lines.findIndex(line => lineKey(line) === lineKey(selected));
  if (selectedIndex < 0) return undefined;
  const isChange = (line: DiffLine) => line.kind === 'addition' || line.kind === 'removal' || (line.kind === 'meta' && line.text === '\\ No newline at end of file');
  let start = selectedIndex, end = selectedIndex + 1;
  while (start > 0 && isChange(lines[start - 1])) start--;
  while (end < lines.length && isChange(lines[end])) end++;
  const block = lines.slice(start, end);
  const beforeRows = block.filter(line => line.kind === 'removal');
  const afterRows = block.filter(line => line.kind === 'addition');
  const segmenter = typeof Intl.Segmenter === 'function' ? new Intl.Segmenter(undefined, { granularity: 'grapheme' }) : undefined;
  const segments = (text: string) => segmenter ? [...segmenter.segment(text)].map(part => part.segment) : Array.from(text);
  const before = segments(beforeRows.map(line => line.text).join('\n'));
  const after = segments(afterRows.map(line => line.text).join('\n'));
  let prefixLength = 0, suffixLength = 0;
  while (prefixLength < Math.min(before.length, after.length) && before[prefixLength] === after[prefixLength]) prefixLength++;
  while (suffixLength < Math.min(before.length, after.length) - prefixLength && before[before.length - 1 - suffixLength] === after[after.length - 1 - suffixLength]) suffixLength++;
  const noNewline = (kind: 'addition' | 'removal') => block.some((line, index) => line.kind === 'meta' && block[index - 1]?.kind === kind);
  return {
    prefix: before.slice(0, prefixLength).join(''), before: before.slice(prefixLength, before.length - suffixLength).join(''),
    after: after.slice(prefixLength, after.length - suffixLength).join(''), suffix: suffixLength ? before.slice(-suffixLength).join('') : '',
    beforeLines: beforeRows.map(line => line.oldLine!), afterLines: afterRows.map(line => line.newLine!),
    beforeNoNewline: noNewline('removal'), afterNoNewline: noNewline('addition'),
  };
}

export function effectValue(effect: StyleEffect, position: number): string {
  const progress = Math.max(0, Math.min(100, position)) / 100;
  if (effect.color) {
    const rgba = effect.color.before.map((value, index) => value + (effect.color!.after[index] - value) * progress);
    return 'rgba(' + rgba.slice(0, 3).map(Math.round).join(', ') + ', ' + Number(rgba[3].toFixed(3)) + ')';
  }
  const size = effect.length!;
  return Number((size.before + (size.after - size.before) * progress).toFixed(3)) + size.unit;
}
