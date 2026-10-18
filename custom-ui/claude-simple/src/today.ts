export interface HistorySlot {
  start: string;
  end: string;
  /** kWh; import, production, charge, charged */
  energy: number;
  /** kWh; export, discharge */
  returnEnergy: number;
}

export interface HistorySeries {
  group: string;
  title?: string;
  isTemp?: boolean;
  data: HistorySlot[];
}

export interface SessionRecord {
  created: string;
  loadpoint: string;
  chargedEnergy: number;
}

export interface CurvePoint {
  /** unix milliseconds, end of the slot */
  ts: number;
  /** cumulative kWh */
  actual: number;
  forecast: number;
}

export interface TodaySummary {
  /** kWh produced since midnight */
  produced: number;
  /** persisted forecast of the finished slots plus the remaining forecast, null without a forecast */
  forecastDay: number | null;
  /** kWh still expected until the end of the day, null without a forecast */
  remaining: number | null;
  /** produced / forecast in % over the slots that have a forecast, null when there is nothing to compare */
  accuracy: number | null;
  curve: CurvePoint[];
  gridImport: number;
  gridExport: number;
  batteryCharge: number;
  batteryDischarge: number;
  carEnergy: number;
  carSessions: number;
  /** end of the last finished slot */
  updated: Date | null;
}

export interface TodayOptions {
  now: Date;
  /** loadpoint titles of heating devices, they are not cars */
  heating: string[];
  /** state.forecast.solar.today.energy */
  remaining?: number | null;
}

/** below this the forecast is too small to judge */
const MIN_COMPARABLE = 0.1;

const num = (v: unknown) => (typeof v === "number" && Number.isFinite(v) ? v : 0);

export function startOfDay(now: Date): Date {
  const d = new Date(now);
  d.setHours(0, 0, 0, 0);
  return d;
}

function sum(slots: HistorySlot[], key: "energy" | "returnEnergy"): number {
  return slots.reduce((s, slot) => s + num(slot[key]), 0);
}

export function summarizeToday(
  series: HistorySeries[] | null,
  sessions: SessionRecord[],
  { now, heating, remaining = null }: TodayOptions
): TodaySummary {
  const from = startOfDay(now).getTime();
  const to = now.getTime();

  // only finished slots of today
  const finished = (s: HistorySeries): HistorySlot[] =>
    (s.data ?? []).filter((slot) => {
      const start = Date.parse(slot.start);
      const end = Date.parse(slot.end);
      return Number.isFinite(start) && Number.isFinite(end) && start >= from && end <= to;
    });

  const all = (series ?? []).map((s) => ({ ...s, data: finished(s) }));
  const group = (name: string) => all.filter((s) => s.group === name).flatMap((s) => s.data);

  const pv = group("pv");
  const forecast = group("forecast");
  const grid = group("grid");
  const battery = group("battery");
  const cars = all
    .filter((s) => s.group === "loadpoint" && !s.isTemp && !heating.includes(s.title ?? ""))
    .flatMap((s) => s.data);

  // sum production per slot, then compare with the forecast of the same slot
  const byStart = (slots: HistorySlot[]) => {
    const m = new Map<number, number>();
    for (const slot of slots) {
      const k = Date.parse(slot.start);
      m.set(k, (m.get(k) ?? 0) + num(slot.energy));
    }
    return m;
  };
  const pvBySlot = byStart(pv);
  const forecastBySlot = byStart(forecast);

  let actual = 0;
  let expected = 0;
  const curve: CurvePoint[] = [];
  for (const start of [...forecastBySlot.keys()].sort((a, b) => a - b)) {
    const f = forecastBySlot.get(start)!;
    // a slot without forecast says nothing about its accuracy
    if (f <= 0) continue;
    actual += pvBySlot.get(start) ?? 0;
    expected += f;
    curve.push({ ts: start + 15 * 60 * 1000, actual, forecast: expected });
  }

  const elapsedForecast = [...forecastBySlot.values()].reduce((a, b) => a + b, 0);
  const left = remaining !== null && Number.isFinite(remaining) ? Math.max(remaining, 0) : null;

  const lastEnd = [...pv, ...forecast, ...grid, ...battery]
    .map((s) => Date.parse(s.end))
    .reduce((a, b) => Math.max(a, b), 0);

  return {
    produced: sum(pv, "energy"),
    forecastDay: left === null ? null : elapsedForecast + left,
    remaining: left,
    accuracy: expected >= MIN_COMPARABLE ? (actual / expected) * 100 : null,
    curve,
    gridImport: sum(grid, "energy"),
    gridExport: sum(grid, "returnEnergy"),
    batteryCharge: sum(battery, "energy"),
    batteryDischarge: sum(battery, "returnEnergy"),
    carEnergy: sum(cars, "energy"),
    carSessions: sessions.filter((s) => {
      const created = Date.parse(s.created);
      return created >= from && created <= to && !heating.includes(s.loadpoint);
    }).length,
    updated: lastEnd > 0 ? new Date(lastEnd) : null,
  };
}
