import { afterEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, readFileSync, rmSync, statSync, truncateSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { serverLog, serverLogPath } from "./server-log.ts";

const directories: string[] = [];
afterEach(() => {
  vi.unstubAllEnvs();
  vi.restoreAllMocks();
  for (const directory of directories.splice(0)) rmSync(directory, { recursive: true, force: true });
});
function setup() {
  const directory = mkdtempSync(join(tmpdir(), "runling-log-"));
  directories.push(directory);
  vi.stubEnv("RUNLING_WEB_CONFIG", join(directory, "runling.config.ts"));
  return directory;
}

describe("server logging", () => {
  it("writes matching console and private file records beside the config", () => {
    const directory = setup();
    const consoleLog = vi.spyOn(console, "error").mockImplementation(() => {});
    serverLog("error", "run.error", { runId: "test", error: new Error("broken") });
    expect(serverLogPath()).toBe(join(directory, ".runling/logs/server.jsonl"));
    const line = readFileSync(serverLogPath(), "utf8").trim();
    expect(consoleLog).toHaveBeenCalledWith(line);
    expect(JSON.parse(line)).toMatchObject({ level: "error", event: "run.error", runId: "test", error: { message: "broken" } });
    expect(statSync(serverLogPath()).mode & 0o777).toBe(0o600);
  });
  it("rotates the file at 10 MiB and keeps appending after rotation", () => {
    setup();
    vi.spyOn(console, "info").mockImplementation(() => {});
    serverLog("info", "first");
    truncateSync(serverLogPath(), 10 * 1024 * 1024);
    serverLog("info", "second");
    serverLog("info", "third");
    expect(statSync(`${serverLogPath()}.1`).size).toBe(10 * 1024 * 1024);
    expect(readFileSync(serverLogPath(), "utf8").trim().split("\n").map(line => JSON.parse(line).event)).toEqual(["second", "third"]);
  });
  it("keeps console logging when the file cannot be written", () => {
    const directory = setup();
    writeFileSync(join(directory, ".runling"), "not a directory");
    const info = vi.spyOn(console, "info").mockImplementation(() => {});
    const error = vi.spyOn(console, "error").mockImplementation(() => {});
    expect(() => serverLog("info", "test")).not.toThrow();
    expect(info).toHaveBeenCalledOnce();
    expect(error).toHaveBeenCalledWith("Cannot write Runling server log:", expect.any(Error));
  });
});

it("does not replace the original server failure when error details are circular", () => {
  setup();
  vi.spyOn(console, "error").mockImplementation(() => {});
  const error: Record<string, unknown> = {};
  error.self = error;
  expect(() => serverLog("error", "http.error", { error })).not.toThrow();
  expect(JSON.parse(readFileSync(serverLogPath(), "utf8"))).toMatchObject({ event: "http.error", message: "Log details could not be serialized" });
});
