import { expect, test, vi } from "vitest";
import { createWorkflowContext, type InputHandler } from "runling";
import { createChattoConversation } from "./conversation.ts";

const options = () => ({
  destination: { roomId: "dm", threadRootId: "root" },
  inbox: { drain: () => [], prepend: () => {}, subscribe: () => () => {} },
  post: vi.fn(async () => {}),
  typing: vi.fn(async () => {}),
});

const question = (signal?: AbortSignal): Parameters<InputHandler>[0] => ({ id: "question", message: "Question?", signal });

test("questions follow queued replies and suppress typing until all questions settle", async () => {
  const sent = Promise.withResolvers<void>();
  const first = Promise.withResolvers<string>();
  const second = Promise.withResolvers<string>();
  const onInput = vi.fn<InputHandler>()
    .mockImplementationOnce(() => first.promise)
    .mockImplementationOnce(() => second.promise);
  const config = options();
  config.post.mockImplementationOnce(() => sent.promise);
  const { ctx, say, agentOptions } = createChattoConversation({ ...createWorkflowContext(), onInput }, config);

  const reply = say("Working");
  const a = ctx.onInput(question());
  const b = ctx.onInput(question());
  await Promise.resolve();
  expect(onInput).not.toHaveBeenCalled();
  sent.resolve();
  await reply;
  await vi.waitFor(() => expect(onInput).toHaveBeenCalledTimes(2));

  await agentOptions.typing!(config.destination, ctx.signal);
  expect(config.typing).not.toHaveBeenCalled();
  first.resolve("one");
  expect(await a).toBe("one");
  await agentOptions.typing!(config.destination, ctx.signal);
  expect(config.typing).not.toHaveBeenCalled();
  second.resolve("two");
  expect(await b).toBe("two");
  await agentOptions.typing!(config.destination, ctx.signal);
  expect(config.typing).toHaveBeenCalledOnce();
});

test("failed posts reach their caller without poisoning later replies", async () => {
  const config = options();
  const error = new Error("offline");
  config.post.mockRejectedValueOnce(error);
  const { say } = createChattoConversation(createWorkflowContext(), config);

  await expect(say("first")).rejects.toBe(error);
  await say("second");
  expect(config.post).toHaveBeenCalledTimes(2);
});

test("a cancelled queued question never reaches the host", async () => {
  const sent = Promise.withResolvers<void>();
  const config = options();
  config.post.mockImplementationOnce(() => sent.promise);
  const onInput = vi.fn<InputHandler>();
  const { ctx, say } = createChattoConversation({ ...createWorkflowContext(), onInput }, config);
  const controller = new AbortController();
  const reply = say("Working");
  const answer = ctx.onInput(question(controller.signal));
  controller.abort(new Error("cancelled"));
  sent.resolve();

  await reply;
  await expect(answer).rejects.toThrow("cancelled");
  expect(onInput).not.toHaveBeenCalled();
});

test("input failure restores typing and separate conversations remain independent", async () => {
  const failed = Promise.withResolvers<string>();
  const config = options();
  const a = createChattoConversation({ ...createWorkflowContext(), onInput: () => failed.promise }, config);
  const b = createChattoConversation(createWorkflowContext(), config);
  const answer = a.ctx.onInput(question());
  await Promise.resolve();
  await b.agentOptions.typing!(config.destination, b.ctx.signal);
  expect(config.typing).toHaveBeenCalledOnce();
  failed.reject(new Error("input failed"));
  await expect(answer).rejects.toThrow("input failed");
  await a.agentOptions.typing!(config.destination, a.ctx.signal);
  expect(config.typing).toHaveBeenCalledTimes(2);
});
