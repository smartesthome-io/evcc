import { reactive } from "vue";
import type { State } from "./types";

export const state = reactive<State & { offline: boolean }>({
  loadpoints: [],
  offline: true,
});

function setProperty(obj: any, props: string[], value: unknown) {
  const prop = props.shift()!;
  if (!props.length) {
    obj[prop] =
      value && typeof value === "object" && !Array.isArray(value)
        ? { ...obj[prop], ...value }
        : value;
    return;
  }
  obj[prop] ??= {};
  setProperty(obj[prop], props, value);
}

function reset() {
  for (const key of Object.keys(state)) {
    delete (state as any)[key];
  }
  Object.assign(state, { loadpoints: [], offline: false });
}

function update(msg: Record<string, unknown>) {
  if (msg["startupCompleted"]) reset();
  for (const [key, value] of Object.entries(msg)) {
    setProperty(state, key.split("."), value);
  }
}

let socket: WebSocket | null = null;
let attempts = 0;
let timer: number | undefined;

/** connects to the evcc websocket; urls are relative so the ui works below any mount path */
export function connect() {
  const url = new URL("ws", window.location.href);
  url.protocol = window.location.protocol === "https:" ? "wss:" : "ws:";

  socket = new WebSocket(url.href);
  socket.onopen = () => {
    attempts = 0;
    state.offline = false;
  };
  socket.onmessage = (evt) => {
    try {
      update(JSON.parse(evt.data));
    } catch (e) {
      console.error("invalid websocket message", e);
    }
  };
  socket.onerror = () => socket?.close();
  socket.onclose = () => {
    state.offline = true;
    window.clearTimeout(timer);
    timer = window.setTimeout(connect, Math.min(1000 * 2 ** attempts++, 10000));
  };
}

export async function post(path: string): Promise<void> {
  const res = await fetch(`api/${path}`, { method: "POST" });
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
}

export async function get<T>(path: string): Promise<T> {
  const res = await fetch(`api/${path}`);
  if (!res.ok) throw new Error(`${path}: ${res.status}`);
  return (await res.json()) as T;
}
