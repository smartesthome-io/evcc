import { describe, expect, test } from "vite-plus/test";
import { summarizeToday, startOfDay, type HistorySeries, type SessionRecord } from "./today";

const NOW = new Date(2026, 5, 15, 12, 7); // 12:07 local, last finished slot ends 12:00
const SLOT = 15 * 60 * 1000;

function slots(values: [number, number?][], from = startOfDay(NOW).getTime()) {
  return values.map(([energy, returnEnergy = 0], i) => ({
    start: new Date(from + i * SLOT).toISOString(),
    end: new Date(from + (i + 1) * SLOT).toISOString(),
    energy,
    returnEnergy,
  }));
}

// 48 slots = until 12:00
const night = (n: number) => Array.from({ length: n }, () => [0] as [number]);
const day = (...v: number[]) => [...night(40), ...v.map((x) => [x] as [number])]; // from 10:00

const series = (
  group: string,
  data: ReturnType<typeof slots>,
  extra: Partial<HistorySeries> = {}
) => ({ group, data, ...extra }) as HistorySeries;

const session = (hour: number, loadpoint = "Garage", chargedEnergy = 5): SessionRecord => ({
  created: new Date(2026, 5, 15, hour).toISOString(),
  loadpoint,
  chargedEnergy,
});

const opts = { now: NOW, heating: ["Heat Pump"] };

describe("summarizeToday", () => {
  test("sums energy per direction", () => {
    const r = summarizeToday(
      [
        series("pv", slots([[1], [2]])),
        series(
          "grid",
          slots([
            [1, 3],
            [2, 4],
          ])
        ),
        series(
          "battery",
          slots([
            [0.5, 0.25],
            [0.5, 0.25],
          ])
        ),
      ],
      [],
      opts
    );
    expect(r.produced).toBe(3);
    expect(r.gridImport).toBe(3);
    expect(r.gridExport).toBe(7);
    expect(r.batteryCharge).toBe(1);
    expect(r.batteryDischarge).toBe(0.5);
  });

  test("ignores slots of other days and unfinished slots", () => {
    const y = startOfDay(NOW).getTime() - SLOT;
    const r = summarizeToday(
      [
        series("pv", [
          ...slots([[10]], y), // yesterday
          ...slots([[1]]), // today, finished
          ...slots([[5]], NOW.getTime() - 60_000), // not finished yet
        ]),
      ],
      [],
      opts
    );
    expect(r.produced).toBe(1);
  });

  test("forecast for today is persisted slots plus remaining", () => {
    const r = summarizeToday([series("forecast", slots([[1], [2], [3]]))], [], {
      ...opts,
      remaining: 10,
    });
    expect(r.forecastDay).toBe(16);
    expect(r.remaining).toBe(10);
  });

  test("no forecast configured", () => {
    const r = summarizeToday([series("pv", slots([[1]]))], [], opts);
    expect(r.forecastDay).toBeNull();
    expect(r.remaining).toBeNull();
    expect(r.accuracy).toBeNull();
    expect(r.curve).toEqual([]);
  });

  test("accuracy compares only slots with a forecast", () => {
    const r = summarizeToday(
      [
        // pv produced 4 kWh, 3 of them in a slot without forecast
        series("pv", slots([[1], [3], [0.5]])),
        series("forecast", slots([[2], [0], [1]])),
      ],
      [],
      opts
    );
    expect(r.accuracy).toBeCloseTo(((1 + 0.5) / (2 + 1)) * 100);
    expect(r.curve.map((p) => [p.actual, p.forecast])).toEqual([
      [1, 2],
      [1.5, 3],
    ]);
    expect(r.produced).toBe(4.5);
  });

  test("accuracy needs a meaningful forecast", () => {
    const r = summarizeToday(
      [series("pv", slots([[0.05]])), series("forecast", slots([[0.05]]))],
      [],
      opts
    );
    expect(r.accuracy).toBeNull();
  });

  test("sums multiple pv and loadpoint series", () => {
    const r = summarizeToday(
      [
        series("pv", slots([[1]]), { title: "East" }),
        series("pv", slots([[2]]), { title: "West" }),
        series("loadpoint", slots([[3]]), { title: "Garage" }),
        series("loadpoint", slots([[4]]), { title: "Carport" }),
      ],
      [],
      opts
    );
    expect(r.produced).toBe(3);
    expect(r.carEnergy).toBe(7);
  });

  test("heating is not car charging", () => {
    const r = summarizeToday(
      [
        series("loadpoint", slots([[3]]), { title: "Garage" }),
        series("loadpoint", slots([[9]]), { title: "Heat Pump" }),
        series("loadpoint", slots([[8]]), { title: "Boiler", isTemp: true }),
      ],
      [session(8), session(9, "Heat Pump")],
      opts
    );
    expect(r.carEnergy).toBe(3);
    expect(r.carSessions).toBe(1);
  });

  test("counts only today's sessions", () => {
    const r = summarizeToday(
      [],
      [
        session(8),
        session(11),
        { ...session(8), created: new Date(2026, 5, 14, 23).toISOString() },
        session(23), // in the future
      ],
      opts
    );
    expect(r.carSessions).toBe(2);
  });

  test("handles empty and invalid data", () => {
    const r = summarizeToday(null, [], { ...opts, remaining: Number.NaN });
    expect(r.produced).toBe(0);
    expect(r.forecastDay).toBeNull();
    expect(r.updated).toBeNull();

    const bad = summarizeToday(
      [series("pv", [{ start: "x", end: "y", energy: NaN, returnEnergy: NaN }])],
      [{ created: "nope", loadpoint: "a", chargedEnergy: NaN }],
      opts
    );
    expect(bad.produced).toBe(0);
    expect(bad.carSessions).toBe(0);
  });

  test("reports the end of the last finished slot", () => {
    const r = summarizeToday([series("pv", slots(day(1, 1)))], [], opts);
    expect(r.updated?.getTime()).toBe(new Date(2026, 5, 15, 10, 30).getTime());
  });
});
