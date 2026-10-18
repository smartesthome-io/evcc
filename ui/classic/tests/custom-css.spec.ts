import { test, expect } from "@playwright/test";
import { start, stop, baseUrl } from "../../../testkit/evcc";

test.use({ baseURL: baseUrl() });

test.beforeAll(async () => {
  await start("basics.evcc.yaml", null, [
    "--disable-auth",
    "--custom-css",
    "ui/classic/tests/custom-css.css",
  ]);
});

test.afterAll(async () => {
  await stop();
});

test.describe("custom-css", async () => {
  test("header and footer are hidden", async ({ page }) => {
    page.on("dialog", () => {
      throw new Error("XSS: inline script detected");
    });
    await page.goto("/");
    await expect(page.getByTestId("header")).toBeHidden();
    await expect(page.getByTestId("footer")).toBeHidden();
  });
});
