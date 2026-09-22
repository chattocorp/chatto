import { afterEach, describe, expect, it, vi } from "vitest";
import { mkdtempSync, readFileSync, rmSync, statSync, truncateSync, writeFileSync } from "node:fs";
import { tmpdir } from "node:os";
import { join } from "node:path";
import { serverLog, serverLogPath } from "./server-log.ts";
import { stripVTControlCharacters } from "node:util";

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
  it("shows the readable reference while retaining the UUID in structured logs", () => {
    setup();
    const output = vi.spyOn(console, "info").mockImplementation(() => {});
    serverLog("info", "run.started", { workflow: "ChattoBot", runId: "uuid", runReference: "brave-otters-4821" });
    expect(stripVTControlCharacters(output.mock.calls[0]![0])).toContain("[brave-otters-4821] ● Started ChattoBot");
    expect(JSON.parse(readFileSync(serverLogPath(), "utf8"))).toMatchObject({ runId: "uuid", runReference: "brave-otters-4821" });
  });
  it("prints readable diagnostics and writes structured private file records beside the config", () => {
    const directory = setup();
    const consoleLog = vi.spyOn(console, "error").mockImplementation(() => {});
    serverLog("error", "run.error", { runId: "test", error: new Error("broken") });
    expect(serverLogPath()).toBe(join(directory, ".runling/logs/server.jsonl"));
    const line = readFileSync(serverLogPath(), "utf8").trim();
    expect(stripVTControlCharacters(consoleLog.mock.calls[0]![0])).toMatch(/^\d{2}:\d{2}:\d{2} \[test\] ✗ run error · broken$/);
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
    expect(error).toHaveBeenCalledWith(expect.stringContaining("Cannot write Runling server log"));
  });
});

it("prints the ready URL once on one colored line and honors NO_COLOR", () => {
  setup();
  vi.stubEnv("NO_COLOR", undefined);
  vi.stubEnv("FORCE_COLOR", "1");
  const output = vi.spyOn(console, "info").mockImplementation(() => {});
  serverLog("info", "server.listening", { url: "http://localhost:5173" });
  const colored = output.mock.calls[0]![0] as string;
  expect(colored).toContain("\x1b[");
  expect(stripVTControlCharacters(colored)).toMatch(/^\d{2}:\d{2}:\d{2} ✓ Runling ready → http:\/\/localhost:5173$/);
  vi.stubEnv("NO_COLOR", "1");
  serverLog("info", "server.listening", { url: "http://localhost:5173" });
  expect(output.mock.calls[1]![0]).not.toContain("\x1b[");
});

it("keeps multiline errors and control sequences out of terminal output", () => {
  setup();
  const output = vi.spyOn(console, "error").mockImplementation(() => {});
  serverLog("error", "run.error", { error: new Error("first\n\x1b[31msecond\rthird") });
  expect(stripVTControlCharacters(output.mock.calls[0]![0])).toMatch(/first second third$/);
  expect(output.mock.calls[0]![0]).not.toContain("\n");
});

it("does not replace the original server failure when error details are circular", () => {
  setup();
  vi.spyOn(console, "error").mockImplementation(() => {});
  const error: Record<string, unknown> = {};
  error.self = error;
  expect(() => serverLog("error", "http.error", { error })).not.toThrow();
  expect(JSON.parse(readFileSync(serverLogPath(), "utf8"))).toMatchObject({ event: "http.error", message: "Log details could not be serialized" });
});
