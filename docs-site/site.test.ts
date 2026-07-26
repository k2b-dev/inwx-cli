import { describe, expect, test } from "bun:test";
import { readdir } from "node:fs/promises";
import { join } from "node:path";
import { createFibelApp } from "@valentinkolb/fibel";
import config from "./fibel.config";

const app = await createFibelApp(config);
const origin = "http://127.0.0.1:4173";

describe("documentation routes", () => {
  test("landing page carries the persistent independence notice", async () => {
    const response = await app.fetch(new Request(`${origin}/en`));
    const html = await response.text();

    expect(response.status).toBe(200);
    expect(html).toContain("Unofficial community project");
    expect(html).toContain("not affiliated with INWX GmbH");
    expect(html).toContain("https://www.inwx.com/");
    expect(html).toContain("https://github.com/k2b-dev/inwx-cli");
    expect(html).toContain("Powered by");
    expect(html).toContain("<pre");
    expect(html).toContain("data-search-open");
    expect(html).toContain("data-theme-toggle");
  });

  test("all local page links resolve", async () => {
    const response = await app.fetch(new Request(`${origin}/en`));
    const html = await response.text();
    const links = [...html.matchAll(/href="(\/[^"#?]*)"/g)].map((match) => match[1]);
    const pages = [...new Set(links)].filter(
      (path) =>
        path !== "/" &&
        !path.startsWith("/_fibel/") &&
        !path.startsWith("/assets/") &&
        path !== "/favicon.ico" &&
        path !== "/favicon.svg",
    );

    for (const path of pages) {
      const linked = await app.fetch(new Request(`${origin}${path}`));
      expect(linked.status, path).toBe(200);
    }
  });

  test("raw Markdown and discovery routes remain available", async () => {
    const raw = await app.fetch(new Request(`${origin}/en/dns-mutations.md`));
    expect(raw.status).toBe(200);
    expect(raw.headers.get("content-type")).toContain("text/markdown");
    expect(await raw.text()).toContain("# Change DNS safely");

    for (const path of ["/llms.txt", "/llms-full.txt", "/en/llms.txt"]) {
      const response = await app.fetch(new Request(`${origin}${path}`));
      expect(response.status, path).toBe(200);
      expect(await response.text()).toContain("inwx");
    }
  });

  test("search finds mutation documentation", async () => {
    const response = await app.fetch(
      new Request(`${origin}/_fibel/search?q=expect&locale=en`),
    );
    const data = (await response.json()) as {
      results: Array<{ href: string; title: string }>;
    };
    expect(response.status).toBe(200);
    expect(data.results.some((result) => result.href === "/en/dns-mutations")).toBeTrue();
  });

  test("light and dark modes render on the server", async () => {
    const light = await app.fetch(new Request(`${origin}/en`));
    expect(await light.text()).toContain('data-theme="light"');

    const dark = await app.fetch(
      new Request(`${origin}/en`, { headers: { Cookie: "fibel_theme=dark" } }),
    );
    const html = await dark.text();
    expect(html).toContain('class="dark"');
    expect(html).toContain('data-theme="dark"');
  });
});

describe("published command contract", () => {
  test("documented command groups match CLI help", () => {
    const root = cliHelp("--help");
    for (const command of [
      "inwx [global flags] version",
      "inwx [global flags] auth check",
      "inwx [global flags] dns zones list",
      "inwx [global flags] dns records list <zone>",
      "inwx [global flags] dns records create <zone>",
      "inwx [global flags] dns records update <zone> --id",
      "inwx [global flags] dns records delete <zone> --id",
    ]) {
      expect(root).toContain(command);
    }

    expect(cliHelp("dns", "records", "create", "--help")).toContain(
      "--expect TOKEN --apply",
    );
    expect(cliHelp("dns", "records", "update", "--help")).toContain(
      "--value-file PATH",
    );
    expect(cliHelp("dns", "records", "delete", "--help")).toContain(
      "--id ID [--expect TOKEN --apply]",
    );
  });

  test("content contains required topics and no private operations", async () => {
    const docsRoot = join(import.meta.dir, "docs", "en");
    const files = (await readdir(docsRoot)).filter((file) => file.endsWith(".md"));
    const content = (
      await Promise.all(files.map((file) => Bun.file(join(docsRoot, file)).text()))
    ).join("\n");
    const normalized = content.replace(/\s+/g, " ");

    for (const required of [
      "INWX_USERNAME_FILE",
      "--environment ote",
      "--expect '<token-from-preview>' --apply",
      "schema_version",
      "Mutation precondition conflict",
      "not affiliated with, endorsed by, maintained by, or supported by INWX GmbH",
    ]) {
      expect(normalized).toContain(required);
    }
    for (const forbidden of ["fd0", "private cluster", "internal hostname"]) {
      expect(content.toLowerCase()).not.toContain(forbidden.toLowerCase());
    }
    expect(content).not.toMatch(/\b[A-Z0-9._%+-]+@[A-Z0-9.-]+\.[A-Z]{2,}\b/i);
    expect(config.siteUrl).toBeUndefined();
  });
});

function cliHelp(...args: string[]) {
  const result = Bun.spawnSync({
    cmd: ["go", "run", "../cmd/inwx", ...args],
    cwd: import.meta.dir,
    stdout: "pipe",
    stderr: "pipe",
    env: { ...process.env, CGO_ENABLED: "0" },
  });
  if (result.exitCode !== 0) {
    throw new Error(result.stderr.toString());
  }
  return result.stdout.toString();
}
