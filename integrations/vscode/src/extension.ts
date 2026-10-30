import * as vscode from 'vscode';
import { readFile, realpath } from 'node:fs/promises';
import { join, relative, isAbsolute } from 'node:path';
import { Engine } from './engine';
import { withinRoot, webviewHTML } from './security';

export function activate(context: vscode.ExtensionContext) {
  const output = vscode.window.createOutputChannel('stef');
  const views = new Set<vscode.Webview>();
  const ready = new Set<vscode.Webview>();
  let engine: Engine | undefined;
  const log = (message: string) => output.appendLine(message.trim());
  function configured() {
    const config = vscode.workspace.getConfiguration('stef');
    const executable = config.get<string>('executablePath') || join(context.extensionPath, 'resources', 'bin', process.platform === 'win32' ? 'stefd.exe' : 'stefd');
    if (!isAbsolute(executable)) throw new Error('stef.executablePath must be an absolute path.');
    return { endpoint: (config.get<string>('endpoint') || 'http://127.0.0.1:4820').replace(/\/$/, ''), executable };
  }
  async function trackedRoot(): Promise<string> {
    if (!vscode.workspace.isTrusted) throw new Error('Trust this workspace before starting the local stef engine.');
    const folders = vscode.workspace.workspaceFolders?.filter((folder) => folder.uri.scheme === 'file') ?? [];
    if (!folders.length) throw new Error('Open a local project folder to use stef.');
    const selected = folders.length === 1 ? folders[0] : await vscode.window.showQuickPick(folders.map((folder) => ({ label: folder.name, folder })), { placeHolder: 'Choose the project stef should track' }).then((item) => item?.folder);
    if (!selected) throw new Error('No project selected.');
    return realpath(selected.uri.fsPath);
  }
  async function start() {
    try {
      const root = await trackedRoot(); const config = configured();
      if (!engine || engine.endpoint !== config.endpoint || engine.executable !== config.executable) {
        await engine?.stop();
        engine = new Engine(config.endpoint, config.executable, context.globalStorageUri.fsPath, log);
        engine.subscribe((message) => { for (const view of views) void view.postMessage(message); });
      }
      await engine.start(root);
      await vscode.commands.executeCommand('stef.workflow.focus');
    } catch (error) { log(String(error)); void vscode.window.showErrorMessage(String(error)); throw error; }
  }
  async function openFile(node: unknown) {
    if (!engine?.root || typeof node !== 'string') return;
    const target = await realpath(withinRoot(engine.root, node));
    const rel = relative(engine.root, target);
    if (rel.startsWith('..') || isAbsolute(rel)) throw new Error('File resolves outside the tracked project.');
    const document = await vscode.workspace.openTextDocument(vscode.Uri.file(target));
    await vscode.window.showTextDocument(document, { preview: true, preserveFocus: false });
  }
  async function attach(webview: vscode.Webview, owner: { onDidDispose: vscode.Event<void> }) {
    const ui = vscode.Uri.joinPath(context.extensionUri, 'resources', 'ui');
    webview.options = { enableScripts: true, localResourceRoots: [ui] };
    views.add(webview);
    const listener = webview.onDidReceiveMessage(async (message) => {
      try {
        if (message?.type === 'stef.ready') {
          ready.add(webview);
          for (const state of engine?.state() ?? [{ type: 'stef.connection', connected: false, detail: 'Start tracking to observe this workspace.' }]) await webview.postMessage(state);
        } else if (message?.type === 'stef.start') await start();
        else if (message?.type === 'stef.openFile') await openFile(message.node);
        else if (message?.type === 'stef.export' && message.session && Array.isArray(message.session.events)) {
          const destination = await vscode.window.showSaveDialog({ defaultUri: vscode.Uri.joinPath(context.globalStorageUri, 'stef-session.json'), filters: { JSON: ['json'] } });
          if (destination) await vscode.workspace.fs.writeFile(destination, Buffer.from(JSON.stringify(message.session, null, 2)));
        }
      } catch (error) { log(String(error)); void vscode.window.showErrorMessage(String(error)); }
    });
    // The view owns this handler and subscription; hidden retained views keep receiving.
    const remove = () => { listener.dispose(); views.delete(webview); ready.delete(webview); };
    context.subscriptions.push({ dispose: remove });
    context.subscriptions.push(owner.onDidDispose(remove));
    try {
      const index = await readFile(join(ui.fsPath, 'index.html'), 'utf8');
      webview.html = webviewHTML(index, webview.cspSource, (path) => webview.asWebviewUri(vscode.Uri.joinPath(ui, path)).toString());
    } catch (error) { log(String(error)); webview.html = '<h2>stef</h2><p>Packaged UI unavailable. Rebuild with make extension.</p>'; }
  }
  const provider: vscode.WebviewViewProvider = { resolveWebviewView(view) { void attach(view.webview, view); } };
  context.subscriptions.push(output,
    vscode.window.registerWebviewViewProvider('stef.workflow', provider, { webviewOptions: { retainContextWhenHidden: true } }),
    vscode.commands.registerCommand('stef.start', start),
    vscode.commands.registerCommand('stef.stop', () => engine?.stop()),
    vscode.commands.registerCommand('stef.openBeside', async () => {
      if (!engine?.connected) await start();
      const panel = vscode.window.createWebviewPanel('stef.editor', 'stef', vscode.ViewColumn.Beside, { enableScripts: true, retainContextWhenHidden: true });
      await attach(panel.webview, panel);
    }),
    vscode.commands.registerCommand('stef.showConfig', async () => {
      const config = configured();
      const client = await vscode.window.showQuickPick(['VS Code (HTTP)', 'Codex (HTTP)', 'Claude / Cursor (stdio)'], { placeHolder: 'Generate configuration to copy; nothing is installed' });
      if (!client) return;
      const content = client.startsWith('Codex') ? '[mcp_servers.stef]\nurl = ' + JSON.stringify(config.endpoint + '/mcp') + '\n' : JSON.stringify(client.startsWith('VS Code') ? { servers: { stef: { type: 'http', url: config.endpoint + '/mcp' } } } : { mcpServers: { stef: { command: config.executable, args: ['--mcp', '--daemon', config.endpoint] } } }, null, 2);
      const document = await vscode.workspace.openTextDocument({ content, language: client.startsWith('Codex') ? 'toml' : 'json' });
      await vscode.window.showTextDocument(document, { preview: false });
    }),
    { dispose: () => { void engine?.stop(); } },
  );
  if (vscode.workspace.getConfiguration('stef').get<boolean>('autoStart') && vscode.workspace.isTrusted) void start().catch(log);
  return { getSnapshot: () => engine?.snapshot, isConnected: () => engine?.connected ?? false, readyViews: () => ready.size };
}
