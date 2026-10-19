import { describe, expect, test } from "vite-plus/test";
import { liveFlow, MIN_LINK } from "./flow";
import type { Loadpoint, State } from "./types";

const labels = {
  pv: "PV",
  gridImport: "Grid",
  batteryDischarge: "Battery",
  home: "Home",
  consumer: "Consumer",
  batteryCharge: "Battery charge",
  loadpoint: "Loadpoint",
  gridExport: "Export",
};

const lp = (chargePower: number, extra: Partial<Loadpoint> = {}): Loadpoint =>
  ({ title: "Garage", chargePower, chargerFeatureHeating: false, ...extra }) as Loadpoint;

const flow = (state: Partial<State>) => liveFlow({ loadpoints: [], ...state }, labels);
const total = (f: ReturnType<typeof flow>, id: string, dir: "source" | "target") =>
  f.links.filter((l) => l[dir] === id).reduce((s, l) => s + l.value, 0);
const link = (f: ReturnType<typeof flow>, source: string, target: string) =>
  f.links.find((l) => l.source === source && l.target === target)?.value;

describe("liveFlow", () => {
  test("sunny day: pv feeds home, car, battery, then exports", () => {
    const f = flow({
      pvPower: 8000,
      homePower: 1000,
      grid: { power: -2000 },
      battery: { power: -2000, soc: 50, capacity: 10 },
      loadpoints: [lp(3000)],
    });
    expect(link(f, "pv", "home")).toBe(1000);
    expect(link(f, "pv", "battery-charge")).toBe(2000);
    expect(link(f, "pv", "lp-0")).toBe(3000);
    expect(link(f, "pv", "grid-export")).toBe(2000);
    expect(f.nodes.find((n) => n.id === "lp-0")?.kind).toBe("car");
  });

  test("night: battery discharges before grid imports", () => {
    const f = flow({
      pvPower: 0,
      homePower: 1500,
      grid: { power: 500 },
      battery: { power: 1000, soc: 40, capacity: 10 },
    });
    expect(link(f, "battery-discharge", "home")).toBe(1000);
    expect(link(f, "grid-import", "home")).toBe(500);
  });

  test("battery never feeds itself and grid never feeds export", () => {
    const f = flow({
      pvPower: 0,
      homePower: 0,
      grid: { power: 3000 },
      battery: { power: -3000, soc: 10, capacity: 10 },
    });
    expect(link(f, "grid-import", "battery-charge")).toBe(3000);
    expect(link(f, "battery-discharge", "battery-charge")).toBeUndefined();
    expect(link(f, "grid-import", "grid-export")).toBeUndefined();
  });

  test("works without battery and without grid", () => {
    const f = flow({ pvPower: 2000, homePower: 2000 });
    expect(f.links).toEqual([{ source: "pv", target: "home", value: 2000 }]);
    expect(f.nodes.map((n) => n.id).sort()).toEqual(["home", "pv"]);
  });

  test("home falls back to the power balance", () => {
    const f = flow({ pvPower: 3000, grid: { power: 1000 }, loadpoints: [lp(1000)] });
    expect(link(f, "pv", "home")).toBe(3000);
    expect(link(f, "grid-import", "lp-0")).toBe(1000);
  });

  test("heating loadpoints are heating nodes", () => {
    const f = flow({
      pvPower: 1000,
      homePower: 0,
      loadpoints: [lp(1000, { title: "Heat pump", chargerFeatureHeating: true })],
    });
    expect(f.nodes.find((n) => n.id === "lp-0")?.kind).toBe("heating");
  });

  test("sub meters are carved out of home and capped", () => {
    const f = flow({
      pvPower: 2000,
      homePower: 1500,
      aux: [{ title: "Boiler", power: 1000 }],
      consumers: [{ title: "Dryer", power: 1000 }],
    });
    expect(link(f, "pv", "home")).toBeUndefined();
    expect(link(f, "pv", "consumer-0")).toBe(1000);
    expect(link(f, "pv", "consumer-1")).toBe(500);
  });

  test("ignores NaN, Infinity and negative loadpoint power", () => {
    const f = flow({
      pvPower: NaN,
      homePower: Infinity,
      grid: { power: NaN },
      battery: { power: Infinity, soc: 0, capacity: 0 },
      loadpoints: [lp(-500), lp(NaN)],
    });
    expect(f.links).toEqual([]);
    expect(f.nodes).toEqual([]);
  });

  test("drops hairline links", () => {
    const f = flow({ pvPower: MIN_LINK - 1, homePower: MIN_LINK - 1 });
    expect(f.links).toEqual([]);
  });

  test("never moves more than a node provides", () => {
    // meter mismatch: sinks want more than the sources deliver
    const f = flow({
      pvPower: 1000,
      homePower: 5000,
      grid: { power: 500 },
      loadpoints: [lp(4000)],
    });
    for (const n of f.nodes) {
      const used = n.side === "source" ? total(f, n.id, "source") : total(f, n.id, "target");
      expect(used).toBeLessThanOrEqual(n.power);
    }
    expect(f.links.reduce((s, l) => s + l.value, 0)).toBeLessThanOrEqual(1500);
  });
});
