import { test, expect } from "@playwright/test";
import { start, stop, baseUrl } from "../../../testkit/evcc";

test.use({ baseURL: baseUrl() });

test.beforeAll(async () => {
  await start("demo.evcc.yaml");
});
test.afterAll(async () => {
  await stop();
});

test.beforeEach(async ({ page }) => {
  await page.goto("/ui/claude-simple/");
});

test.describe("now", () => {
  test("status badge shows the most important state with the others as chips", async ({ page }) => {
    const status = page.getByRole("status");
    await expect(status).toContainText("Charging car");
    await expect(status).toContainText("Charging home battery");
    await expect(status).toContainText("Grid export");
  });

  test("energy flow is drawn", async ({ page }) => {
    await expect(page.getByRole("region", { name: "Energy flow" })).toBeVisible();
    await expect(page.getByText("No energy flow right now")).toBeHidden();
  });

  test("home battery shows charge level", async ({ page }) => {
    const battery = page.getByRole("region", { name: "Home battery" });
    await expect(battery).toContainText("64");
    await expect(battery).toContainText("Charging");
  });

  test("charging point shows vehicle and power", async ({ page }) => {
    const charging = page.getByRole("region", { name: "Charging", exact: true });
    await expect(charging).toContainText("Garage");
    await expect(charging).toContainText("Blue Car");
    await expect(charging).toContainText("3.0 kW");
    await expect(charging).toContainText("42 %");
  });

  test("heating shows temperature and power", async ({ page }) => {
    const heating = page.getByRole("region", { name: "Heating" });
    await expect(heating).toContainText("Heat Pump");
    await expect(heating).toContainText("800 W");
    await expect(heating).toContainText("45 °C");
  });

  test("charge mode can be changed", async ({ page }) => {
    const charging = page.getByRole("region", { name: "Charging", exact: true });
    await expect(charging.getByRole("button", { name: "Smart", pressed: true })).toBeVisible();
    await charging.getByRole("button", { name: "Fast" }).click();
    await expect(charging.getByRole("button", { name: "Fast", pressed: true })).toBeVisible();
  });
});

test.describe("navigation", () => {
  test("tabs switch the view", async ({ page }) => {
    const nav = page.getByRole("navigation", { name: "Main" });
    await expect(nav.getByRole("link", { name: "Now" })).toHaveAttribute("aria-current", "page");

    await nav.getByRole("link", { name: "Config" }).click();
    await expect(page).toHaveURL(/#\/config$/);
    await expect(page.getByRole("link", { name: "Open the full configuration" })).toBeVisible();

    await nav.getByRole("link", { name: "Now" }).click();
    await expect(page.getByRole("status")).toBeVisible();
  });

  test("the chosen ui is remembered for the root url", async ({ page }) => {
    await page.goto("/");
    await expect(page).toHaveURL(/\/ui\/claude-simple\//);

    await page.getByRole("link", { name: "Classic interface" }).click();
    await expect(page).toHaveURL(/\/ui\/classic\//);

    await page.goto("/");
    await expect(page).not.toHaveURL(/claude-simple/);
  });
});
