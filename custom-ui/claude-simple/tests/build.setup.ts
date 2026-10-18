import { execFileSync } from "child_process";
import { test } from "@playwright/test";

test("build claude-simple", () => {
  execFileSync("vp", ["build", "custom-ui/claude-simple"], { stdio: "inherit" });
});
