import { expect, test } from "vitest";
import { readFile } from "node:fs/promises";
import { parse } from "yaml";
import { assertUnusedVersion } from "../scripts/check-release.mjs";

test("rejects both available and unpublished historical versions", () => {
  const metadata = {
    versions: { "0.3.0": {} },
    time: { "0.1.0": "2017-08-29" },
  };
  expect(() => assertUnusedVersion("0.3.0", metadata)).toThrow("already been published");
  expect(() => assertUnusedVersion("0.1.0", metadata)).toThrow("already been published");
  expect(() => assertUnusedVersion("0.3.1", metadata)).not.toThrow();
});

test("publishes only tested tag artifacts with a dedicated OIDC permission", async () => {
  const workflow = parse(await readFile(
    new URL("../../../.github/workflows/publish-runling.yml", import.meta.url), "utf8",
  ));
  expect(workflow.on.push).toEqual({ tags: ["runling/v*"] });
  expect(workflow.permissions).toEqual({ contents: "read" });
  expect(workflow.jobs.publish.needs).toBe("package");
  expect(workflow.jobs.publish.if).toBe("github.event_name == 'push'");
  expect(workflow.jobs.publish.environment).toBe("npm");
  expect(workflow.jobs.publish.permissions["id-token"]).toBe("write");
  const steps = workflow.jobs.package.steps;
  for (const command of [
    "mise x -- node packages/runling/scripts/check-release.mjs",
    "mise check-runling", "mise test-runling", "mise test-runling-package",
  ]) {
    expect(steps.some((step: { run?: string }) => step.run === command)).toBe(true);
  }
  const consumerTest = steps.find((step: { run?: string }) => step.run === "mise test-runling-package");
  expect(consumerTest.env.RUNLING_RELEASE_DIR).toBe("${{ runner.temp }}/runling-release");
  expect(workflow.jobs.publish.steps.at(-1).run).toContain('npm publish "${packages[0]}"');
  expect(workflow.jobs.publish.steps.at(-1).run).toContain("--ignore-scripts");
});

test("keeps Runling versioning independent of Chatto prereleases", async () => {
  const readJson = async (path: string) => JSON.parse(
    await readFile(new URL(path, import.meta.url), "utf8"),
  );
  const config = await readJson("../../../.release-please-config.json");
  const manifest = await readJson("../../../.release-please-manifest.json");
  const pkg = await readJson("../package.json");
  expect(config.packages["packages/runling"]).toMatchObject({
    "release-type": "simple",
    "package-name": "runling",
    "component": "runling",
    "versioning": "default",
    "prerelease": false,
    "draft": false,
    "include-component-in-tag": true,
    "tag-separator": "/",
    "extra-files": [{ type: "json", path: "package.json", jsonpath: "$.version" }],
  });
  expect(config.packages["."]["exclude-paths"]).toContain("packages/runling");
  expect(manifest["packages/runling"]).toBe(pkg.version);
  expect((await readFile(new URL("../version.txt", import.meta.url), "utf8")).trim()).toBe(pkg.version);
  expect(pkg.repository).toEqual({
    type: "git",
    url: "git+https://github.com/chattocorp/chatto.git",
    directory: "packages/runling",
  });
});
