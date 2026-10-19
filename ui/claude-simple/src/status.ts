import type { State } from "./types";

export type SiteStatus =
  | "charging-car"
  | "discharging-battery"
  | "charging-battery"
  | "grid-export"
  | "grid-import"
  | "idle";

/** highest priority first */
export const STATUS_PRIORITY: SiteStatus[] = [
  "charging-car",
  "discharging-battery",
  "charging-battery",
  "grid-export",
  "grid-import",
];

// a state turns on above ON and only turns off below OFF, so the badge does not flicker
export const ON_THRESHOLD = 100;
export const OFF_THRESHOLD = 50;

export interface SiteStatusResult {
  primary: SiteStatus;
  /** other active states, highest priority first */
  chips: SiteStatus[];
}

function num(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

function powers(state: State): Record<Exclude<SiteStatus, "idle">, number> {
  const car = (state.loadpoints ?? [])
    .filter((lp) => !lp.chargerFeatureHeating)
    .reduce((sum, lp) => sum + Math.max(num(lp.chargePower), 0), 0);
  const battery = num(state.battery?.power);
  const grid = num(state.grid?.power);
  return {
    "charging-car": car,
    "discharging-battery": Math.max(battery, 0),
    "charging-battery": Math.max(-battery, 0),
    "grid-export": Math.max(-grid, 0),
    "grid-import": Math.max(grid, 0),
  };
}

export function siteStatus(state: State, previous?: SiteStatusResult): SiteStatusResult {
  const p = powers(state);
  const wasActive = new Set<SiteStatus>(previous ? [previous.primary, ...previous.chips] : []);

  const active = STATUS_PRIORITY.filter((s) => {
    const threshold = wasActive.has(s) ? OFF_THRESHOLD : ON_THRESHOLD;
    return p[s as keyof typeof p] > threshold;
  });

  // importing is only a fallback title, not worth a chip next to a more specific state
  const specific = active.filter((s) => s !== "grid-import");
  if (specific.length === 0) {
    return { primary: active[0] ?? "idle", chips: [] };
  }
  return { primary: specific[0]!, chips: specific.slice(1) };
}
