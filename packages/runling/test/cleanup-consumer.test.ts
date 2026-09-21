import { EventEmitter } from "node:events";
import { expect, test, vi, afterEach } from "vitest";
import { rm } from "node:fs/promises";
import { cleanupConsumer, stopConsumer } from "../scripts/cleanup-consumer.mjs";

vi.mock("node:fs/promises", () => ({ rm: vi.fn().mockResolvedValue(undefined) }));
afterEach(() => vi.restoreAllMocks());

test("cleanup retries transient recursive deletion errors", async () => {
  await cleanupConsumer("/tmp/consumer-test");
  expect(rm).toHaveBeenCalledWith("/tmp/consumer-test", {
    recursive: true, force: true, maxRetries: 10, retryDelay: 100,
  });
});

test.skipIf(process.platform === "win32")("waits for close, not npm exit, and stops remaining descendants", async () => {
  const server = Object.assign(new EventEmitter(), { pid: 12345 });
  const kill = vi.spyOn(process, "kill").mockImplementation(() => true);
  const stopped = stopConsumer(server, 1000);
  server.emit("exit", 0);
  await Promise.resolve();
  expect(kill).toHaveBeenCalledTimes(1);
  expect(kill).toHaveBeenCalledWith(-12345, "SIGTERM");
  server.emit("close", 0);
  await stopped;
  expect(kill).toHaveBeenLastCalledWith(-12345, "SIGKILL");
  expect(server.listenerCount("close")).toBe(0);
});

test.skipIf(process.platform === "win32")("forces shutdown after the grace period", async () => {
  const server = Object.assign(new EventEmitter(), { pid: 12345 });
  const kill = vi.spyOn(process, "kill").mockImplementation(() => true);
  await stopConsumer(server, 1);
  expect(kill).toHaveBeenLastCalledWith(-12345, "SIGKILL");
  expect(server.listenerCount("close")).toBe(0);
});

test.skipIf(process.platform === "win32")("accepts an already-stopped process group", async () => {
  const server = Object.assign(new EventEmitter(), { pid: 12345 });
  vi.spyOn(process, "kill").mockImplementation(() => {
    throw Object.assign(new Error("No such process"), { code: "ESRCH" });
  });
  await expect(stopConsumer(server, 1)).resolves.toBeUndefined();
  expect(server.listenerCount("close")).toBe(0);
});
