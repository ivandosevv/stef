const assert = require('node:assert/strict');
const vscode = require('vscode');

async function until(condition) { const deadline = Date.now() + 20000; while (Date.now() < deadline) { if (condition()) return; await new Promise((done) => setTimeout(done, 50)); } throw new Error('Timed out waiting for docked stef view'); }
exports.run = async () => {
  const endpoint = 'http://127.0.0.1:4823';
  await vscode.workspace.getConfiguration('stef').update('endpoint', endpoint, vscode.ConfigurationTarget.Workspace);
  const extension = vscode.extensions.getExtension('ivandosevv.stef'); assert.ok(extension);
  const api = await extension.activate();
  try {
    await vscode.commands.executeCommand('stef.start');
    await until(() => api.isConnected() && api.readyViews() >= 1 && api.getSnapshot().repo === process.env.STEF_TEST_ROOT);
    assert.equal(api.getSnapshot().repo, process.env.STEF_TEST_ROOT);
    await vscode.commands.executeCommand('stef.openBeside');
    await until(() => api.readyViews() >= 2);
    const response = await fetch(endpoint + '/api/activity', { method: 'POST', headers: { 'Content-Type': 'application/json' }, body: JSON.stringify({ agent_id: 'ide-test', phase: 'editing', node_name: 'main.go', tool: 'editor' }) });
    assert.equal(response.status, 201);
    await until(() => api.getSnapshot().events.some((event) => event.agent_id === 'ide-test'));
    await vscode.commands.executeCommand('stef.stop');
    assert.equal(api.isConnected(), false);
    console.log('stef IDE smoke: sidebar, editor pane, packaged UI handshake, live activity and shutdown passed.');
  } finally { await vscode.commands.executeCommand('stef.stop'); }
};
