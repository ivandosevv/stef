import { isIP } from 'node:net';
import { resolve, relative, isAbsolute } from 'node:path';
import { randomBytes } from 'node:crypto';

export function localEndpoint(value: string): URL {
  const url = new URL(value);
  const host = url.hostname.replace(/^\[|\]$/g, '');
  const loopback = host === 'localhost' || host === '::1' || (isIP(host) === 4 && host.startsWith('127.'));
  if (url.protocol !== 'http:' || !loopback || url.username || url.password || url.search || url.hash || url.pathname !== '/') throw new Error('stef endpoint must be an HTTP loopback origin.');
  return url;
}
export function withinRoot(root: string, node: string): string {
  const file = node.split('#')[0];
  if (!file || isAbsolute(file) || file.includes('\0') || file.split(/[\\/]/).includes('..') || file.includes(':')) throw new Error('Invalid project path.');
  const target = resolve(root, file); const rel = relative(root, target);
  if (!rel || rel.startsWith('..') || isAbsolute(rel)) throw new Error('Path is outside the tracked project.');
  return target;
}
export function webviewHTML(index: string, cspSource: string, resource: (path: string) => string): string {
  const nonce = randomBytes(18).toString('hex');
  const policy = "default-src 'none'; img-src " + cspSource + " data:; style-src " + cspSource + " 'unsafe-inline'; script-src 'nonce-" + nonce + "'; connect-src 'none'; base-uri 'none'; frame-src 'none';";
  const html = index.replace(/(src|href)="(?:\.\/|\/)([^\"]+)"/g, (_match, attr, path: string) => {
    if (path.includes('..') || path.includes(':')) throw new Error('Invalid packaged asset path.');
    return attr + '="' + resource(path) + '"';
  }).replace(/<script\b/g, '<script nonce="' + nonce + '"');
  return html.replace('<head>', '<head><meta http-equiv="Content-Security-Policy" content="' + policy + '">');
}
