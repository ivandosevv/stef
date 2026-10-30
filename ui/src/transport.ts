import type { StreamMessage } from './lib';

interface HostAPI { postMessage(message: unknown): void; }
type HostWindow = Window & { stefHost?: HostAPI; acquireVsCodeApi?: () => HostAPI };
export const isEmbedded = (): boolean => Boolean((window as HostWindow).stefHost) || typeof (window as HostWindow).acquireVsCodeApi === 'function';
let hostAPI: HostAPI | undefined;
function host(): HostAPI | undefined {
  if (!hostAPI) hostAPI = (window as HostWindow).stefHost ?? (window as HostWindow).acquireVsCodeApi?.();
  return hostAPI;
}
export function sendToHost(message: unknown): void { host()?.postMessage(message); }

export function connectStream(onmessage: (message: StreamMessage) => void, onstatus: (connected: boolean, detail: string) => void, sessionID = ''): () => void {
  if (isEmbedded()) {
    const receive = (event: MessageEvent) => {
      const data = event.data;
      if (data?.type === 'stef.stream') onmessage(data.message as StreamMessage);
      else if (data?.type === 'stef.connection') onstatus(Boolean(data.connected), String(data.detail ?? ''));
    };
    window.addEventListener('message', receive);
    sendToHost({ type: 'stef.ready', session_id: sessionID });
    return () => window.removeEventListener('message', receive);
  }
  let socket: WebSocket | undefined;
  let retry: ReturnType<typeof setTimeout>;
  let disposed = false;
  function connect() {
    if (disposed) return;
    socket = new WebSocket((location.protocol === 'https:' ? 'wss://' : 'ws://') + location.host + '/ws' + (sessionID ? '?session_id=' + encodeURIComponent(sessionID) : ''));
    socket.onopen = () => onstatus(true, '');
    socket.onmessage = (message) => {
      try { onmessage(JSON.parse(message.data) as StreamMessage); }
      catch { onstatus(true, 'The engine sent an unreadable event.'); }
    };
    socket.onclose = () => { if (disposed) return; onstatus(false, 'Connecting to the local engine…'); if (!disposed) retry = setTimeout(connect, 1500); };
    socket.onerror = () => socket?.close();
  }
  connect();
  return () => { disposed = true; clearTimeout(retry); socket?.close(); };
}
