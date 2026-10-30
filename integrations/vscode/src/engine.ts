import { spawn, type ChildProcess } from 'node:child_process';
import { realpath, mkdir } from 'node:fs/promises';
import { createHash } from 'node:crypto';
import { join } from 'node:path';
import WebSocket from 'ws';
import { localEndpoint } from './security';

export type Snapshot = { type: 'snapshot'; repo: string; events: Record<string, unknown>[] };
export type BridgeMessage = { type: 'stef.stream'; message: Snapshot | { type: 'event'; event: Record<string, unknown> } } | { type: 'stef.connection'; connected: boolean; detail: string };

export class Engine {
  snapshot: Snapshot = { type: 'snapshot', repo: '', events: [] };
  connected = false;
  root = '';
  private process: ChildProcess | undefined;
  private socket: WebSocket | undefined;
  private retry: ReturnType<typeof setTimeout> | undefined;
  private subscribers = new Set<(message: BridgeMessage) => void>();
  private active = false;
  private starting: Promise<void> | undefined;
  constructor(readonly endpoint: string, readonly executable: string, private storage: string, private log: (line: string) => void) { localEndpoint(endpoint); }
  subscribe(listener: (message: BridgeMessage) => void): () => void { this.subscribers.add(listener); return () => this.subscribers.delete(listener); }
  state(): BridgeMessage[] { return [{ type: 'stef.stream', message: this.snapshot }, { type: 'stef.connection', connected: this.connected, detail: this.active ? '' : 'Start tracking to observe this workspace.' }]; }
  private emit(message: BridgeMessage) { for (const subscriber of this.subscribers) subscriber(message); }
  async start(root: string): Promise<void> {
    if (this.starting) return this.starting;
    this.starting = this.doStart(root).finally(() => this.starting = undefined);
    return this.starting;
  }
  private async health(): Promise<{ name: string; repo: string } | undefined> {
    try { const response = await fetch(this.endpoint + '/healthz', { signal: AbortSignal.timeout(1500), redirect: 'error' }); if (response.ok) return await response.json() as { name: string; repo: string }; } catch { /* The engine may not be running yet. */ }
    return undefined;
  }
  private async doStart(root: string): Promise<void> {
    const selected = await realpath(root);
    if (this.active) { if (this.root !== selected) throw new Error('Stop stef before choosing another project.'); return; }
    const health = await this.health();
    if (health && (health.name !== 'stef' || health.repo !== selected)) throw new Error('This port belongs to another project or service. Choose a different stef.endpoint.');
    this.root = selected;
    if (!health) {
      const data = join(this.storage, createHash('sha256').update(selected).digest('hex').slice(0, 16)); await mkdir(data, { recursive: true });
      const url = localEndpoint(this.endpoint);
      const host = url.hostname === 'localhost' ? '127.0.0.1' : url.hostname.replace(/^\[|\]$/g, '');
      const child = spawn(this.executable, ['--repo', selected, '--port', url.port || '80', '--host', host, '--data-dir', data], { stdio: ['ignore', 'ignore', 'pipe'], windowsHide: true });
      this.process = child;
      let launchError: Error | undefined;
      child.on('error', (error) => launchError = error);
      child.stderr?.on('data', (chunk: Buffer) => this.log(chunk.toString()));
      try {
        const deadline = Date.now() + 20000;
        while (Date.now() < deadline) {
          if (launchError) throw launchError;
          if (child.exitCode !== null) throw new Error('The stef engine exited during startup. See the stef output channel.');
          const ready = await this.health();
          if (ready) { if (ready.name !== 'stef' || ready.repo !== selected) throw new Error('The engine port was claimed by another workspace.'); break; }
          await new Promise((done) => setTimeout(done, 100));
        }
        if (!await this.health()) throw new Error('The stef engine did not become ready.');
      } catch (error) { child.kill(); this.process = undefined; throw error; }
    }
    this.active = true; this.connect();
  }
  private connect() {
    if (!this.active) return;
    const url = localEndpoint(this.endpoint); url.protocol = 'ws:'; url.pathname = '/ws';
    const socket = new WebSocket(url, { maxPayload: 64 * 1024 * 1024 }); this.socket = socket;
    socket.on('open', () => { this.connected = true; this.emit({ type: 'stef.connection', connected: true, detail: '' }); });
    socket.on('message', (raw) => {
      try {
        const message = JSON.parse(raw.toString());
        if (message.type === 'snapshot' && (message.events === undefined || Array.isArray(message.events))) {
          if (message.repo !== this.root) { void this.stop(); this.log('Engine changed project; tracking disconnected.'); return; }
          this.snapshot = { type: 'snapshot', repo: message.repo, events: message.events ?? [] };
        } else if (message.type === 'event' && message.event?.id) { this.snapshot = { ...this.snapshot, events: [...this.snapshot.events, message.event].slice(-5000) }; }
        else return;
        this.emit({ type: 'stef.stream', message });
      } catch (error) { this.log('Unreadable engine message: ' + String(error)); }
    });
    socket.on('error', (error) => this.log(error.message));
    socket.on('close', () => { this.connected = false; this.emit({ type: 'stef.connection', connected: false, detail: this.active ? 'Reconnecting to the local engine…' : 'Tracking stopped.' }); if (this.active) this.retry = setTimeout(() => this.connect(), 1500); });
  }
  async stop(): Promise<void> {
    this.active = false; clearTimeout(this.retry); this.socket?.terminate(); this.socket = undefined; this.connected = false;
    const child = this.process; this.process = undefined;
    if (child && child.exitCode === null) { await new Promise<void>((done) => { const timer = setTimeout(() => { child.kill('SIGKILL'); done(); }, 3000); child.once('exit', () => { clearTimeout(timer); done(); }); child.kill('SIGTERM'); }); }
    this.emit({ type: 'stef.connection', connected: false, detail: 'Tracking stopped.' });
  }
}
