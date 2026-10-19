import { describe, expect, test } from "vite-plus/test";
import { siteStatus, OFF_THRESHOLD, ON_THRESHOLD } from "./status";
import type { Loadpoint, State } from "./types";

const lp = (chargePower: number, heating = false) =>
  ({ chargePower, chargerFeatureHeating: heating }) as Loadpoint;

const state = (s: Partial<State>): State => ({ loadpoints: [], ...s });

describe("siteStatus", () => {
  test.each([
    ["idle", state({}), "idle", []],
    ["car", state({ loadpoints: [lp(3000)] }), "charging-car", []],
    ["export", state({ grid: { power: -2000 } }), "grid-export", []],
    ["import", state({ grid: { power: 2000 } }), "grid-import", []],
    [
      "charging battery",
      state({ battery: { power: -1000, soc: 1, capacity: 1 } }),
      "charging-battery",
      [],
    ],
    [
      "discharging battery",
      state({ battery: { power: 1000, soc: 1, capacity: 1 } }),
      "discharging-battery",
      [],
    ],
    [
      "car beats everything, others become chips",
      state({
        loadpoints: [lp(3000)],
        battery: { power: 1000, soc: 1, capacity: 1 },
        grid: { power: -500 },
      }),
      "charging-car",
      ["discharging-battery", "grid-export"],
    ],
    [
      "discharge beats charge beats export",
      state({ battery: { power: -1000, soc: 1, capacity: 1 }, grid: { power: -1000 } }),
      "charging-battery",
      ["grid-export"],
    ],
    [
      "import is no chip next to a specific state",
      state({ grid: { power: 3000 }, loadpoints: [lp(2000)] }),
      "charging-car",
      [],
    ],
    ["heating is not a car", state({ loadpoints: [lp(2000, true)] }), "idle", []],
    ["bad numbers are idle", state({ grid: { power: NaN }, loadpoints: [lp(NaN)] }), "idle", []],
  ])("%s", (_, s, primary, chips) => {
    expect(siteStatus(s)).toEqual({ primary, chips });
  });

  test("hysteresis keeps a state between the thresholds", () => {
    const between = (ON_THRESHOLD + OFF_THRESHOLD) / 2;
    const s = state({ grid: { power: -between } });
    expect(siteStatus(s).primary).toBe("idle");
    expect(siteStatus(s, { primary: "grid-export", chips: [] }).primary).toBe("grid-export");
  });

  test("a state turns off below the lower threshold", () => {
    const s = state({ grid: { power: -(OFF_THRESHOLD - 1) } });
    expect(siteStatus(s, { primary: "grid-export", chips: [] }).primary).toBe("idle");
  });
});
