import { createApp, reactive, h, defineComponent } from "vue";

type State = Record<string, any>;
const state = reactive<State>({});
const status = reactive({ online: false });

// ws keys are dotted paths like "loadpoints.0.chargePower"; values replace whatever sits at the path
const setPath = (target: State, path: string, value: unknown) => {
  const keys = path.split(".");
  const last = keys.pop() as string;
  let node: any = target;
  keys.forEach((key, i) => {
    const next = i + 1 < keys.length ? keys[i + 1] : last;
    node[key] = node[key] ?? (/^\d+$/.test(next) ? [] : {});
    node = node[key];
  });
  node[last] = value;
};

const merge = (target: State, patch: State) => {
  for (const [key, value] of Object.entries(patch)) setPath(target, key, value);
};

const connect = () => {
  const url = new URL("ws", location.href);
  url.protocol = url.protocol === "https:" ? "wss:" : "ws:";
  const ws = new WebSocket(url);
  ws.onopen = () => (status.online = true);
  ws.onclose = () => {
    status.online = false;
    setTimeout(connect, 2000);
  };
  ws.onmessage = (e) => merge(state, JSON.parse(e.data));
};

const kw = (w?: number) => `${((w ?? 0) / 1000).toFixed(1)} kW`;

const App = defineComponent({
  render() {
    const loadpoints: State[] = Array.isArray(state.loadpoints) ? state.loadpoints : [];
    return h("main", { style: "font-family:system-ui;max-width:32rem;margin:2rem auto;padding:0 1rem" }, [
      h("h1", "evcc"),
      h("p", status.online ? "connected" : "offline"),
      h("ul", [
        h("li", `Grid: ${kw(state.grid?.power)}`),
        h("li", `PV: ${kw(state.pvPower)}`),
        h("li", `Home: ${kw(state.homePower)}`),
        h("li", `Battery: ${kw(state.battery?.power)} (${Math.round(state.battery?.soc ?? 0)}%)`),
      ]),
      ...loadpoints.map((lp, i) =>
        h("p", `${lp.title ?? `Loadpoint ${i + 1}`}: ${lp.charging ? "charging" : "idle"}, ${kw(lp.chargePower)}, mode ${lp.mode}`)
      ),
    ]);
  },
});

connect();
createApp(App).mount("#app");
