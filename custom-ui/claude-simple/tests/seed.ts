// builds sqlite statements with a plausible day of energy history up to `now`,
// so the Today view has data without waiting for slots to be persisted

const SLOT = 15 * 60 * 1000;

const ENTITIES = [
  { id: 1, group: "home", name: "home", title: "home", isTemp: 0 },
  { id: 2, group: "loadpoint", name: "lp-1", title: "Garage", isTemp: 0 },
  { id: 3, group: "loadpoint", name: "lp-2", title: "Heat Pump", isTemp: 1 },
  { id: 4, group: "grid", name: "grid", title: "grid", isTemp: 0 },
  { id: 5, group: "pv", name: "pv", title: "pv", isTemp: 0 },
  { id: 6, group: "forecast", name: "forecast", title: "forecast", isTemp: 0 },
  { id: 7, group: "battery", name: "battery", title: "battery", isTemp: 0 },
];

const round = (v: number) => Math.round(v * 1000) / 1000;

export interface SlotValues {
  pv: number;
  forecast: number;
  home: number;
  car: number;
  heat: number;
  gridImport: number;
  gridExport: number;
  batteryCharge: number;
  batteryDischarge: number;
}

/** energy in kWh for the 15 minute slot starting at the given local hour */
export function slotValues(hour: number): SlotValues {
  const kwh = (kw: number) => kw * 0.25;
  const sun = hour >= 6 && hour <= 20 ? Math.max(0, Math.sin((Math.PI * (hour - 6)) / 14)) : 0;
  const pv = kwh(7.5 * sun);
  const forecast = kwh(7.5 * sun * 1.15);
  const home = kwh(0.4 + (hour >= 6 && hour < 9 ? 0.8 : 0) + (hour >= 17 && hour < 22 ? 1.2 : 0));
  const car = hour >= 10 && hour < 12.5 ? kwh(3) : 0;
  const heat = kwh(0.8);

  let surplus = pv - home - car - heat;
  const batteryCharge = hour >= 9 && hour < 14 && surplus > 0 ? Math.min(surplus, kwh(2)) : 0;
  surplus -= batteryCharge;
  const batteryDischarge = hour >= 18 && hour < 23 ? Math.min(kwh(1), home) : 0;
  const deficit = home + car + heat + batteryCharge - pv - batteryDischarge;

  return {
    pv,
    forecast,
    home,
    car,
    heat,
    gridImport: Math.max(deficit, 0),
    gridExport: Math.max(-deficit, 0),
    batteryCharge,
    batteryDischarge,
  };
}

/** SQL that creates the entities and the meter slots of today until the last completed slot */
export function todaySeedSql(now = new Date()): string {
  const midnight = new Date(now);
  midnight.setHours(0, 0, 0, 0);
  const lastSlot = Math.floor(now.getTime() / SLOT) * SLOT;

  const sql = [
    "CREATE TABLE IF NOT EXISTS `entities` (`id` integer PRIMARY KEY AUTOINCREMENT,`group` text,`name` text,`title` text,`is_temp` numeric,`energy_meter` real,`return_energy_meter` real);",
    "CREATE UNIQUE INDEX IF NOT EXISTS `entities_group_name` ON `entities`(`group`,`name`);",
    "CREATE TABLE IF NOT EXISTS `meters` (`meter` integer,`ts` integer,`energy` real,`return_energy` real,`soc_temp` real,`recovered` numeric);",
    "CREATE UNIQUE INDEX IF NOT EXISTS `meters_meter_ts` ON `meters`(`meter`,`ts`);",
  ];

  sql.push(
    "CREATE TABLE IF NOT EXISTS `sessions` (`id` integer PRIMARY KEY AUTOINCREMENT,`created` datetime,`finished` datetime,`loadpoint` text,`identifier` text,`vehicle` text,`odometer` real,`meter_start_kwh` real,`meter_end_kwh` real,`charged_kwh` real,`charge_duration` integer,`soc_start` real,`soc_end` real,`added_range` real,`solar_percentage` real,`price` real,`price_per_kwh` real,`co2_per_kwh` real,`reference_price_per_kwh` real,`reference_co2_per_kwh` real);"
  );

  for (const e of ENTITIES) {
    sql.push(
      `INSERT INTO entities (id, \`group\`, name, title, is_temp) VALUES (${e.id}, '${e.group}', '${e.name}', '${e.title}', ${e.isTemp});`
    );
  }

  const insert = (meter: number, ts: number, energy: number, returnEnergy = 0) =>
    sql.push(
      `INSERT INTO meters (meter, ts, energy, return_energy, recovered) VALUES (${meter}, ${ts / 1000}, ${round(energy)}, ${round(returnEnergy)}, 0);`
    );

  for (let t = midnight.getTime(); t < lastSlot; t += SLOT) {
    const d = new Date(t);
    const v = slotValues(d.getHours() + d.getMinutes() / 60);
    insert(1, t, v.home);
    insert(2, t, v.car);
    insert(3, t, v.heat);
    insert(4, t, v.gridImport, v.gridExport);
    insert(5, t, v.pv);
    insert(6, t, v.forecast);
    insert(7, t, v.batteryCharge, v.batteryDischarge);
  }

  // sessions: a car and a heat pump today, a car yesterday
  const at = (day: number, hour: number) => {
    const d = new Date(midnight);
    d.setDate(d.getDate() + day);
    d.setHours(hour);
    return d;
  };
  const stamp = (d: Date) => d.toISOString().replace("T", " ").replace("Z", "+00:00");
  const session = (start: Date, end: Date, loadpoint: string, kwh: number) => {
    if (end.getTime() > now.getTime()) return;
    sql.push(
      `INSERT INTO sessions (created, finished, loadpoint, vehicle, charged_kwh) VALUES ('${stamp(start)}', '${stamp(end)}', '${loadpoint}', 'Blue Car', ${kwh});`
    );
  };
  session(at(0, 10), at(0, 12), "Garage", 7.5);
  session(at(0, 6), at(0, 8), "Heat Pump", 1.6);
  session(at(-1, 10), at(-1, 12), "Garage", 6);

  return sql.join("\n") + "\n";
}

if (process.argv[1]?.endsWith("seed.ts")) {
  process.stdout.write(todaySeedSql());
}
