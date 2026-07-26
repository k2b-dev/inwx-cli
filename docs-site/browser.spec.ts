import { expect, test, type Page } from "@playwright/test";

function failOnConsoleErrors(page: Page): string[] {
  const errors: string[] = [];
  page.on("console", (message) => {
    if (message.type() === "error") errors.push(message.text());
  });
  page.on("pageerror", (error) => errors.push(error.message));
  return errors;
}

test("desktop navigation, search, theme, raw text, and links work", async ({
  page,
  request,
}) => {
  const errors = failOnConsoleErrors(page);

  await page.goto("/");
  await expect(page.getByRole("heading", { name: "inwx CLI" })).toBeVisible();
  await expect(
    page.getByText(
      "This project is not affiliated with, endorsed by, maintained by, or supported by INWX GmbH.",
    ),
  ).toBeVisible();

  await page.getByRole("link", { name: "Installation", exact: true }).first().click();
  await expect(page).toHaveURL(/\/installation$/);
  await expect(
    page.getByText(
      "curl -fsSL https://raw.githubusercontent.com/k2b-dev/inwx-cli/main/scripts/install.sh | sh",
      { exact: true },
    ),
  ).toBeVisible();

  await page.getByRole("button", { name: /Search docs/ }).click();
  await page.getByPlaceholder("Search documentation...").fill("expect token");
  await expect(page.getByText("Change DNS safely", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");

  await expect(page.locator("html")).toHaveClass(/light/);
  await page.getByRole("button", { name: "Toggle theme" }).first().click();
  await expect(page.locator("html")).toHaveClass(/dark/);

  const official = page.getByRole("link", {
    name: "Unofficial community project — not affiliated with INWX GmbH",
  });
  await expect(official).toHaveAttribute("href", "https://www.inwx.com/");

  const raw = await request.get("/en/installation.md");
  expect(raw.ok()).toBeTruthy();
  expect(await raw.text()).toContain("# Installation");
  const discovery = await request.get("/llms.txt");
  expect(discovery.ok()).toBeTruthy();
  expect(await discovery.text()).toContain("[Installation](/en/installation.md)");

  expect(errors).toEqual([]);
});

test("mobile navigation exposes the complete documentation", async ({ page }) => {
  const errors = failOnConsoleErrors(page);
  await page.setViewportSize({ width: 390, height: 844 });
  await page.goto("/");

  await page.getByRole("button", { name: "Open navigation" }).click();
  const navigationLink = page
    .getByRole("navigation")
    .getByRole("link", { name: "Authentication", exact: true });
  await expect(navigationLink).toBeVisible();
  await navigationLink.click();
  await expect(page).toHaveURL(/\/authentication$/);
  await expect(
    page.getByRole("heading", { name: "Authentication", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("link", {
      name: "Unofficial community project — not affiliated with INWX GmbH",
    }),
  ).toBeVisible();

  expect(errors).toEqual([]);
});
