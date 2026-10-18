import type { State } from "./types";

export type NodeKind =
  | "pv"
  | "grid-import"
  | "battery-discharge"
  | "home"
  | "consumer"
  | "car"
  | "heating"
  | "battery-charge"
  | "grid-export";

export interface FlowNode {
  id: string;
  label: string;
  kind: NodeKind;
  side: "source" | "sink";
  /** W */
  power: number;
}

export interface FlowLink {
  source: string;
  target: string;
  /** W */
  value: number;
}

export interface Flow {
  nodes: FlowNode[];
  links: FlowLink[];
}

/** links below this are dropped, they would render as hairlines */
export const MIN_LINK = 10;

function num(v: unknown): number {
  return typeof v === "number" && Number.isFinite(v) ? v : 0;
}

/**
 * Builds the live power flow. Sources feed sinks by priority: PV, then battery, then grid
 * into home, battery charge, loadpoints, export. Battery does not feed itself, grid does not
 * feed export. Power no counterpart can explain (meter mismatch) is dropped, not invented.
 */
export function liveFlow(state: State, labels: Record<string, string>): Flow {
  const pv = Math.max(num(state.pvPower ?? sum(state.pv?.map((m) => m.power))), 0);
  const gridPower = num(state.grid?.power);
  const batteryPower = num(state.battery?.power);
  const loadpoints = state.loadpoints ?? [];

  const lpPower = loadpoints.map((lp) => Math.max(num(lp.chargePower), 0));
  const lpTotal = sum(lpPower);

  const gridImport = Math.max(gridPower, 0);
  const gridExport = Math.max(-gridPower, 0);
  const discharge = Math.max(batteryPower, 0);
  const charge = Math.max(-batteryPower, 0);

  // whole house load without loadpoints, falls back to the power balance
  const balance = Math.max(pv + gridPower + batteryPower - lpTotal, 0);
  let home = state.homePower !== undefined ? Math.max(num(state.homePower), 0) : balance;

  // sub meters are part of home; cap them so they never exceed it
  const subMeters = [...(state.aux ?? []), ...(state.consumers ?? [])];
  const consumers: { id: string; label: string; power: number }[] = [];
  let remaining = home;
  subMeters.forEach((m, i) => {
    const power = Math.min(Math.max(num(m.power), 0), remaining);
    remaining -= power;
    consumers.push({ id: `consumer-${i}`, label: m.title || m.name || labels["consumer"]!, power });
  });
  home = remaining;

  const nodes: FlowNode[] = [
    { id: "pv", label: labels["pv"]!, kind: "pv", side: "source", power: pv },
    {
      id: "grid-import",
      label: labels["gridImport"]!,
      kind: "grid-import",
      side: "source",
      power: gridImport,
    },
    {
      id: "battery-discharge",
      label: labels["batteryDischarge"]!,
      kind: "battery-discharge",
      side: "source",
      power: discharge,
    },
    { id: "home", label: labels["home"]!, kind: "home", side: "sink", power: home },
    ...consumers.map((c) => ({
      id: c.id,
      label: c.label,
      kind: "consumer" as const,
      side: "sink" as const,
      power: c.power,
    })),
    {
      id: "battery-charge",
      label: labels["batteryCharge"]!,
      kind: "battery-charge",
      side: "sink",
      power: charge,
    },
    ...loadpoints.map((lp, i) => ({
      id: `lp-${i}`,
      label: lp.title || `${labels["loadpoint"]} ${i + 1}`,
      kind: (lp.chargerFeatureHeating ? "heating" : "car") as NodeKind,
      side: "sink" as const,
      power: lpPower[i]!,
    })),
    {
      id: "grid-export",
      label: labels["gridExport"]!,
      kind: "grid-export",
      side: "sink",
      power: gridExport,
    },
  ];

  const sources = nodes.filter((n) => n.side === "source");
  const sinks = nodes.filter((n) => n.side === "sink");
  const left = new Map(nodes.map((n) => [n.id, n.power]));

  const links: FlowLink[] = [];
  for (const s of sources) {
    for (const t of sinks) {
      // battery does not feed itself, grid does not feed export
      if (s.id === "battery-discharge" && t.id === "battery-charge") continue;
      if (s.id === "grid-import" && t.id === "grid-export") continue;

      const value = Math.min(left.get(s.id)!, left.get(t.id)!);
      if (value <= 0) continue;
      left.set(s.id, left.get(s.id)! - value);
      left.set(t.id, left.get(t.id)! - value);
      if (value >= MIN_LINK) links.push({ source: s.id, target: t.id, value });
    }
  }

  // only nodes that take part are drawn
  const used = new Set(links.flatMap((l) => [l.source, l.target]));
  return { nodes: nodes.filter((n) => used.has(n.id)), links };
}

function sum(values: number[] | undefined): number {
  return (values ?? []).reduce((a, b) => a + num(b), 0);
}
