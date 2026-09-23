import { input, Type } from "runling";
import { createChattoWebhook, postToChatto, messageSignal, type ChattoPost } from "./chatto/webhook.ts";
export { createChattoPoster } from "./chatto/webhook.ts";

export interface DemoOptions {
  post: ChattoPost;
  /** Timeout for each question, in seconds. */
  timeout?: number;
}

export function createChattoInputDemo({ post, timeout = 30 }: DemoOptions) {
  const timeoutMs = Math.ceil(timeout * 1000);
  if (!Number.isFinite(timeout) || timeout < 0 || timeoutMs > 2_147_483_647) {
    throw new RangeError("timeout must be seconds between 0 and 2147483.647");
  }
  return createChattoWebhook({
    name: "Chatto input demo",
    output: Type.Object({ name: Type.String(), topic: Type.String() }),
    post,
    async run(ctx, _delivery, destination) {
      const name = await input(ctx, `What is your name? Please reply within ${timeout} seconds.`, { timeout });
      const topic = await input(ctx, `What would you like to work on? Please reply within ${timeout} seconds.`, { timeout });
      await post(destination, `Thanks! Your name: ${name}\nYour topic: ${topic}`, messageSignal(ctx));
      return { name, topic };
    },
  });
}

export default createChattoInputDemo({ post: postToChatto });
