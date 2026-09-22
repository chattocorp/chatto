import { expect, test, vi } from "vitest";
import { createChattoClient, RealtimeEvent, RoomKind } from "./index.js";

const event = () => new RealtimeEvent({ id: "incoming", actorId: "human", event: {
  case: "messagePosted", value: { roomId: "room", roomKind: RoomKind.CHANNEL,
    bodyPlaintext: "hello", threadRootEventId: "root", inReplyTo: "target" },
} });
function setup(response: unknown = {}) {
  const request = vi.fn<typeof fetch>().mockImplementation(async () => Response.json(response));
  return { request, client: createChattoClient({ serverUrl: "https://chat.example", apiKey: "secret", fetch: request }) };
}

test("viewer identity and normalized message reads use the authenticated client transport", async () => {
  const { client, request } = setup({ user: { profile: { id: "viewer" } } });
  expect(await client.getViewer()).toEqual({ id: "viewer" });
  expect(String(request.mock.calls[0]![0])).toBe("https://chat.example/api/connect/chatto.api.v1.ViewerService/GetViewer");
  request.mockImplementation(async () => Response.json({ message: {
    id: "target", roomId: "room", actorId: "viewer", threadRootEventId: "root", inReplyTo: "earlier", body: "",
  } }));
  expect(await client.getMessage({ roomId: "room", messageId: "target" })).toEqual({
    id: "target", roomId: "room", authorId: "viewer", threadRootId: "root", inReplyTo: "earlier", body: "",
  });
  expect(JSON.parse(request.mock.calls[1]![1]!.body as string)).toEqual({ roomId: "room", eventId: "target" });
  expect(request.mock.calls[1]![1]).toMatchObject({ redirect: "error", headers: { Authorization: "Bearer secret" } });
});

test("rejects missing viewer identity", async () => {
  const { client } = setup({ user: { profile: {} } });
  await expect(client.getViewer()).rejects.toThrow("viewer identity");
});

test("DM and viewer mention recognition needs no lookup and preserves empty text", async () => {
  const { client, request } = setup();
  const incoming = event();
  if (incoming.event.case !== "messagePosted") throw new Error("fixture");
  incoming.event.value.roomKind = RoomKind.DM;
  incoming.event.value.bodyPlaintext = "";
  expect(await client.addressedMessage(incoming, { viewerId: "viewer" })).toMatchObject({
    id: "incoming", authorId: "human", body: "", roomId: "room", threadRootId: "root", reasons: ["direct_message"],
  });
  incoming.event.value.roomKind = RoomKind.CHANNEL;
  incoming.event.value.mentions = [{ includesViewer: true }] as typeof incoming.event.value.mentions;
  expect((await client.addressedMessage(incoming, { viewerId: "viewer" }))?.reasons).toEqual(["mention"]);
  expect(request).not.toHaveBeenCalled();
});

test.each(["self", "missing-text", "missing-id", "missing-actor", "other-event", "unaddressed"])("ignores %s without lookup", async kind => {
  const { client, request } = setup();
  const incoming = event();
  if (incoming.event.case !== "messagePosted") throw new Error("fixture");
  if (kind === "self") incoming.actorId = "viewer";
  if (kind === "missing-text") incoming.event.value.bodyPlaintext = undefined;
  if (kind === "missing-id") incoming.id = "";
  if (kind === "missing-actor") incoming.actorId = "";
  if (kind === "unaddressed") incoming.event.value.inReplyTo = "";
  if (kind === "other-event") incoming.event = { case: undefined };
  expect(await client.addressedMessage(incoming, { viewerId: "viewer" })).toBeUndefined();
  expect(request).not.toHaveBeenCalled();
});

test.each(["valid", "wrong-author", "wrong-room", "wrong-id", "wrong-thread", "missing"])("reply ownership verification: %s", async kind => {
  const { client, request } = setup({ message: kind === "missing" ? undefined : {
    id: kind === "wrong-id" ? "different" : "target", roomId: kind === "wrong-room" ? "different" : "room",
    actorId: kind === "wrong-author" ? "human" : "viewer", threadRootEventId: kind === "wrong-thread" ? "different" : "root",
  } });
  const result = await client.addressedMessage(event(), { viewerId: "viewer" });
  if (kind === "valid") expect(result?.reasons).toEqual(["reply"]);
  else expect(result).toBeUndefined();
  expect(request).toHaveBeenCalledOnce();
});

test("replies to a viewer-authored thread root are recognized", async () => {
  const { client } = setup({ message: { id: "target", roomId: "room", actorId: "viewer" } });
  const incoming = event();
  if (incoming.event.case !== "messagePosted") throw new Error("fixture");
  incoming.event.value.threadRootEventId = "target";
  expect((await client.addressedMessage(incoming, { viewerId: "viewer" }))?.reasons).toEqual(["reply"]);
});

test("lookup failures propagate sanitized errors; cancellation does not become an ignored message", async () => {
  const { client, request } = setup();
  request.mockRejectedValue(new Error("private URL or token"));
  await expect(client.addressedMessage(event(), { viewerId: "viewer" })).rejects.toThrow(/^Chatto API request did not complete$/);
  const controller = new AbortController();
  controller.abort(new Error("cancelled"));
  await expect(client.addressedMessage(event(), { viewerId: "viewer", signal: controller.signal })).rejects.toThrow("cancelled");
  expect(request).toHaveBeenCalledOnce();
});
