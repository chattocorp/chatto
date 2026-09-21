import { expect, test } from "vitest";
import { eventStream } from "./stream.ts";

test("sends snapshots and removes listeners when the browser disconnects", async () => {
  let released = false;
  let emit: (event: string, data: unknown) => void = () => {};
  const response = eventStream(
    new Request("http://localhost/events"),
    (send) => {
      emit = send;
      send("snapshot", { status: "running" });
      return () => {
        released = true;
      };
    },
  );
  expect(response.headers.get("content-type")).toBe("text/event-stream");
  const reader = response.body!.getReader();
  expect(new TextDecoder().decode((await reader.read()).value)).toContain(
    'event: snapshot\ndata: {"status":"running"}',
  );
  emit("record", { type: "finished" });
  expect(new TextDecoder().decode((await reader.read()).value)).toContain(
    '"type":"finished"',
  );
  await reader.cancel();
  expect(released).toBe(true);
  expect(() => emit("record", {})).not.toThrow();
});
