const { test } = require('node:test');
const assert = require('node:assert/strict');
const { mkdir, mkdtemp, rm } = require('node:fs/promises');
const { resolve, join } = require('node:path');
const { createServer } = require('node:net');
const { execFileSync } = require('node:child_process');
const { Engine } = require('../out/engine');

async function port() { const server = createServer(); await new Promise((done) => server.listen(0, '127.0.0.1', done)); const value = server.address().port; await new Promise((done) => server.close(done)); return value; }
async function until(condition) { const deadline = Date.now() + 10000; while (Date.now() < deadline) { if (condition()) return; await new Promise((done) => setTimeout(done, 25)); } throw new Error('Timed out waiting for engine event'); }

test('the extension starts the real engine, receives hooks, reuses matching roots, and stops only its own process', async () => {
  const project = resolve('../..'); await mkdir(join(project, '.test-workspace'), { recursive: true });
  const root = await mkdtemp(join(project, '.test-workspace', 'engine.'));
  execFileSync('git', ['init', '-q', root]);
  const endpoint = 'http://127.0.0.1:' + await port();
  const owner = new Engine(endpoint, join(project, 'bin/stefd'), join(root, '.stef'), () => {});
  const borrowed = new Engine(endpoint, join(project, 'bin/stefd'), join(root, '.stef'), () => {});
  const wrong = new Engine(endpoint, join(project, 'bin/stefd'), join(root, '.stef'), () => {});
  try {
    await owner.start(root); await until(() => owner.connected && owner.snapshot.repo === root);
    await borrowed.start(root); await until(() => borrowed.connected);
    await borrowed.stop();
    assert.equal((await fetch(endpoint + '/healthz')).status, 200);
    await assert.rejects(wrong.start(project), /another project/);
    const response = await fetch(endpoint + '/api/activity', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ agent_id: 'adapter-test', phase: 'testing', tool: 'go-test' }) });
    assert.equal(response.status, 201);
    await until(() => owner.snapshot.events.some((event) => event.phase === 'testing'));
    assert.equal(owner.snapshot.events.at(-1).source, 'hook');
    await owner.stop();
    await assert.rejects(fetch(endpoint + '/healthz'));
  } finally { await borrowed.stop(); await owner.stop(); await wrong.stop(); await rm(root, { recursive: true, force: true }); }
});
