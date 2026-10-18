import fs from "fs";
import os from "os";
import path from "path";
import { test, expect } from "@playwright/test";
import { start, stop, baseUrl } from "../../../testkit/evcc";
import { slotValues, todaySeedSql } from "./seed";

test.use({ baseURL: baseUrl(), locale: "en-US" });

// fixed day so the seeded history does not depend on when the test runs
const NOW = new Date(2026, 5, 15, 15, 7);
const SLOT_HOURS = 0.25;

function sums() {
  const total = {
    pv: 0,
    forecast: 0,
    car: 0,
    gridImport: 0,
    gridExport: 0,
    batteryCharge: 0,
    batteryDischarge: 0,
  };
  for (let h = 0; h < 15; h += SLOT_HOURS) {
    const v = slotValues(h);
    for (const key of Object.keys(total) as (keyof typeof total)[]) total[key] += v[key];
  }
  return total;
}

// same rounding as the ui: two decimals below 10 kWh, one below 100
const kwh = (v: number) => `${v.toFixed(v >= 100 ? 0 : v >= 10 ? 1 : 2)} kWh`;

test.beforeAll(async () => {
  const file = path.join(os.tmpdir(), `claude-simple-today-${process.pid}.sql`);
  fs.writeFileSync(file, todaySeedSql(NOW));
  await start("demo.evcc.yaml", file);
});
test.afterAll(async () => {
  await stop();
});

test.beforeEach(async ({ page }) => {
  await page.clock.setFixedTime(NOW);
  await page.goto("/ui/claude-simple/#/today");
});

test.describe("today", () => {
  test("solar shows production and forecast accuracy", async ({ page }) => {
    const solar = page.getByRole("region", { name: "Solar" });
    await expect(solar).toContainText(kwh(sums().pv));
    // forecast is 15 % above production
    await expect(solar).toContainText("87 %");
    await expect(solar).toContainText("13 % below forecast");
    // no forecast provider is configured, so there is no day total
    await expect(solar).toContainText("No solar forecast available");
  });

  test("car charging shows sessions and energy", async ({ page }) => {
    const car = page.getByRole("region", { name: "Car charging" });
    // the heat pump session and yesterday's session do not count
    await expect(car.getByText("Sessions")).toBeVisible();
    await expect(car).toContainText(/Sessions\s*1(?!\d)/);
    await expect(car).toContainText(kwh(sums().car));
  });

  test("grid and battery show daily energy", async ({ page }) => {
    const total = sums();
    const grid = page.getByRole("region", { name: "Grid" });
    await expect(grid).toContainText(kwh(total.gridImport));
    await expect(grid).toContainText(kwh(total.gridExport));

    const battery = page.getByRole("region", { name: "Home battery" });
    await expect(battery).toContainText(kwh(total.batteryCharge));
    await expect(battery).toContainText(kwh(total.batteryDischarge));
  });

  test("shows when the data was updated", async ({ page }) => {
    await expect(page.getByText(/Updated 0?3:00 PM/)).toBeVisible();
  });
});
