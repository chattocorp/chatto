import { describe, expect, test } from "vitest";
import { createExec, CommandError } from "./shell.ts";
import { observeRunlingEvents, type RunlingEvent } from "./events.ts";
import { mkdtemp, realpath, rm } from "node:fs/promises";
import { tmpdir } from "node:os";
import { join } from "node:path";

const printArgs = "process.stdout.write(JSON.stringify(process.argv.slice(1)))";

describe("createExec", () => {
  test("passes arguments literally without requiring shell syntax", async () => {
    const values = [
      "a b",
      "it's a quote",
      "$HOME",
      "$(echo injected)",
      ";",
      "|",
      ">",
      "",
      "line\nbreak",
    ];
    expect(
      await createExec({ cwd: import.meta.dirname })`${process.execPath} -e ${printArgs} ${values}`.json(),
    ).toEqual(values);
  });

  test("joins adjacent interpolations into one argument", async () => {
    expect(
      await createExec({ cwd: import.meta.dirname })`${process.execPath} -e ${printArgs} refs/${"feature a"}:${"target b"}`.json(),
    ).toEqual(["refs/feature a:target b"]);
  });

  test("preserves output buffers, newlines, and nonzero exit codes", async () => {
    const events: RunlingEvent[] = [];
    const command =
      "console.log('stdout'); console.error('stderr'); process.exit(7)";
    await observeRunlingEvents(
      (event) => events.push(event),
      async () => {
        const result =
          await createExec({ cwd: import.meta.dirname })`${process.execPath} -e ${command}`.nothrow();
        expect(result).toMatchObject({
          stdout: Buffer.from("stdout\n"),
          stderr: Buffer.from("stderr\n"),
          exitCode: 7,
        });
      },
    );
    expect(
      events.find((event) => event.type === "command.finished"),
    ).toMatchObject({
      status: "failed",
      output: { stdout: "stdout\n", stderr: "stderr\n" },
    });
    await expect(
      createExec({ cwd: import.meta.dirname })`${process.execPath} -e ${command}`,
    ).rejects.toBeInstanceOf(CommandError);
  });

  test("reports missing executables as failed even in nothrow mode", async () => {
    const events: RunlingEvent[] = [];
    await observeRunlingEvents(
      (event) => events.push(event),
      async () => {
        const result =
          await createExec({ cwd: import.meta.dirname })`runling-command-that-does-not-exist`.nothrow();
        expect(result.exitCode).not.toBe(0);
      },
    );
    expect(
      events.find((event) => event.type === "command.finished"),
    ).toMatchObject({ status: "failed" });
  });

  test("keeps explicit directories isolated across concurrent commands", async () => {
    const directories = await Promise.all([1, 2].map(() => mkdtemp(join(tmpdir(), "runling-exec-"))));
    try {
      const exec = createExec();
      const script = "setTimeout(() => process.stdout.write(process.cwd()), 5)";
      const results = await Promise.all(directories.map(async directory =>
        realpath(await exec`${process.execPath} -e ${script}`.cwd(directory).text()),
      ));
      expect(results).toEqual(await Promise.all(directories.map(directory => realpath(directory))));
      await expect(exec`${process.execPath} -e ${script}`).rejects.toThrow("An explicit directory is required");
    } finally {
      await Promise.all(directories.map(directory => rm(directory, { recursive: true, force: true })));
    }
  });
});
