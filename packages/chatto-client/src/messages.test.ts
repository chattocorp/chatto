import { expect, test, vi } from "vitest";
import { createChattoClient } from "./index.js";

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
