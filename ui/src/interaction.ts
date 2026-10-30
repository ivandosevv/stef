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
  if (namedColors[lower]) return namedColors[lower];
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
  const limit = sizeLimits[property];
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

function declarations(lines: DiffLine[], kind: 'addition' | 'removal'): Map<string, string[]> {
  const values = new Map<string, string[]>();
  for (const line of lines.filter(line => line.kind === kind)) {
    // Only literal declarations from removed/added source, never current files or evaluated source.
    for (const match of line.text.matchAll(/(?:^|[;{])\s*([\w-]+)\s*:\s*([^;{}]+?)\s*(?=;|}|$)/g)) {
      const property = match[1].startsWith('--') ? match[1] : match[1].toLowerCase();
      values.set(property, [...(values.get(property) ?? []), match[2].trim()]);
    }
  }
  return values;
}

// Each sample represents one unambiguous paired declaration, not a reconstruction of the app.
export function styleEffects(diff: string, path: string): StyleEffect[] {
  if (!/\.(?:css|scss|less|svelte|html)$/i.test(path)) return [];
  const effects: StyleEffect[] = [];
  for (const hunk of diffHunks(diff)) {
    const before = declarations(hunk, 'removal'), after = declarations(hunk, 'addition');
    for (const [property, oldValues] of before) {
      const newValues = after.get(property);
      if (oldValues.length !== 1 || newValues?.length !== 1) continue;
      const effect = safeEffect(property, oldValues[0], newValues[0]);
      if (effect) effects.push(effect);
    }
  }
  return effects.slice(0, 12);
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
