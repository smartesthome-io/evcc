import { test, expect } from "@playwright/test";
import { start, stop, baseUrl } from "../../../testkit/evcc";

test.use({ baseURL: baseUrl() });

test.beforeAll(async () => {
  await start("basics.evcc.yaml");
});
test.afterAll(async () => {
  await stop();
});

test.describe("ui mount", async () => {
  test("classic works from its mount", async ({ page }) => {
    await page.goto("/ui/classic/");
    await expect(page.getByRole("heading", { name: "Hello World" })).toBeVisible();
    await expect(page.getByTestId("visualization")).toContainText("1.0 kW");
  });

  test("mount without trailing slash redirects", async ({ page }) => {
    await page.goto("/ui/classic");
    await expect(page).toHaveURL(/\/ui\/classic\/(#\/)?$/);
    await expect(page.getByRole("heading", { name: "Hello World" })).toBeVisible();
  });

  test("config route works from mount", async ({ page }) => {
    await page.goto("/ui/classic/#/config");
    await expect(page.getByRole("heading", { name: "Configuration" })).toBeVisible();
  });
});
