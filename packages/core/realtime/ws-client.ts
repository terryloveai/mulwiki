export type RealtimeEvent = {
  type: string;
  workspace_id?: string;
  agent_id?: string;
  task_id?: string;
  payload?: unknown;
  timestamp?: string;
};

export type RealtimeHandler = (payload: unknown) => void;
export type RealtimeAnyHandler = (event: RealtimeEvent) => void;

export type RealtimeClientOptions = {
  workspace: string;
  agentId?: string;
  url?: string;
  reconnectBaseMs?: number;
  reconnectMaxMs?: number;
};

export class RealtimeClient {
  private socket: WebSocket | null = null;
  private reconnectTimer: ReturnType<typeof setTimeout> | null = null;
  private reconnectAttempt = 0;
  private closed = false;
  private readonly handlers = new Map<string, Set<RealtimeHandler>>();
  private readonly anyHandlers = new Set<RealtimeAnyHandler>();

  constructor(private readonly options: RealtimeClientOptions) {
    this.connect();
  }

  on(eventType: string, handler: RealtimeHandler): () => void {
    const handlers = this.handlers.get(eventType) ?? new Set<RealtimeHandler>();
    handlers.add(handler);
    this.handlers.set(eventType, handlers);

    return () => {
      handlers.delete(handler);
      if (handlers.size === 0) this.handlers.delete(eventType);
    };
  }

  onAny(handler: RealtimeAnyHandler): () => void {
    this.anyHandlers.add(handler);
    return () => this.anyHandlers.delete(handler);
  }

  close() {
    this.closed = true;
    if (this.reconnectTimer) {
      clearTimeout(this.reconnectTimer);
      this.reconnectTimer = null;
    }
    this.socket?.close();
    this.socket = null;
  }

  private connect() {
    if (this.closed || typeof window === "undefined" || typeof WebSocket === "undefined") return;

    const socket = new WebSocket(this.buildURL());
    this.socket = socket;

    socket.onopen = () => {
      this.reconnectAttempt = 0;
    };

    socket.onmessage = (message) => {
      const event = this.parseEvent(message.data);
      if (!event) return;
      this.dispatch(event);
    };

    socket.onclose = () => {
      if (this.socket === socket) this.socket = null;
      if (!this.closed) this.scheduleReconnect();
    };

    socket.onerror = () => {
      socket.close();
    };
  }

  private buildURL() {
    if (this.options.url) return this.options.url;

    const protocol = window.location.protocol === "https:" ? "wss" : "ws";
    const params = new URLSearchParams({ workspace: this.options.workspace });
    if (this.options.agentId) params.set("agent_id", this.options.agentId);
    return `${protocol}://${window.location.host}/ws?${params.toString()}`;
  }

  private scheduleReconnect() {
    if (this.reconnectTimer) return;

    const base = this.options.reconnectBaseMs ?? 500;
    const max = this.options.reconnectMaxMs ?? 15_000;
    const delay = Math.min(max, base * 2 ** this.reconnectAttempt);
    this.reconnectAttempt += 1;

    this.reconnectTimer = setTimeout(() => {
      this.reconnectTimer = null;
      this.connect();
    }, delay);
  }

  private parseEvent(data: unknown): RealtimeEvent | null {
    if (typeof data !== "string") return null;

    try {
      const event = JSON.parse(data) as Partial<RealtimeEvent>;
      if (!event || typeof event.type !== "string") return null;
      return event as RealtimeEvent;
    } catch {
      return null;
    }
  }

  private dispatch(event: RealtimeEvent) {
    for (const handler of this.anyHandlers) {
      handler(event);
    }

    const handlers = this.handlers.get(event.type);
    if (!handlers) return;
    const payload = event.payload ?? event;
    for (const handler of handlers) {
      handler(payload);
    }
  }
}
