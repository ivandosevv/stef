const { test } = require('node:test');
const assert = require('node:assert/strict');
const { localEndpoint, withinRoot, webviewHTML } = require('../out/security');

test('IDE endpoint and file operations remain confined to the local project', () => {
  for (const origin of ['https://localhost:4820', 'http://remote.example', 'http://localhost:4820/api', 'http://user:pass@127.0.0.1:4820', 'http://127.0.0.1:4820?x=1']) assert.throws(() => localEndpoint(origin));
  assert.equal(localEndpoint('http://127.0.0.1:4820').hostname, '127.0.0.1');
  assert.equal(withinRoot('/personal/project', 'src/main.go#Run'), '/personal/project/src/main.go');
  for (const node of ['../other/file.go', '/etc/passwd', 'src/../../outside', 'C:\\outside']) assert.throws(() => withinRoot('/personal/project', node));
});

test('the embedded panel uses packaged assets, nonced scripts, and no direct network', () => {
  const html = webviewHTML('<html><head></head><body><script type="module" src="./assets/app.js"></script><link href="./assets/app.css"></body></html>', 'https://local.vscode-cdn.net', (path) => 'https://local.vscode-cdn.net/' + path);
  assert.match(html, /connect-src 'none'/);
  assert.match(html, /script nonce="[a-f0-9]+"/);
  assert.match(html, /src="https:\/\/local.vscode-cdn.net\/assets\/app.js"/);
  assert.throws(() => webviewHTML('<head><script src="./../outside.js"></script></head>', 'host', String));
});
