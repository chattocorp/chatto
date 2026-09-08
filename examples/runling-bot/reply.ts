import { readFile } from "node:fs/promises";
import { Type, task } from "runling";
import { generateReply } from "./agent.ts";
import { createReplySender } from "./sender.ts";
import { startTyping } from "./typing.ts";

/** The same v1 body is used for direct mentions and direct messages. */
export const webhookInput = Type.Object({
  version: Type.Literal(1),
  id: Type.String({ minLength: 1 }),
  type: Type.Literal("message.created"),
  triggers: Type.Array(
    Type.Union([Type.Literal("mention"), Type.Literal("direct_message")]),
    { minItems: 1 },
  ),
  occurred_at: Type.String(),
  bot_id: Type.String({ minLength: 1 }),
  room_id: Type.String({ minLength: 1 }),
  thread_root_id: Type.Union([Type.String({ minLength: 1 }), Type.Null()]),
  message: Type.Object({
    id: Type.String({ minLength: 1 }),
    author_id: Type.String({ minLength: 1 }),
    body: Type.String(),
  }),
});

interface BotConfig {
  serverUrl: string;
  apiKey: string;
}

/** Read credentials at execution time. Never include them in workflow output. */
async function readEnvironment(): Promise<BotConfig> {
  const serverUrl = process.env.CHATTO_RUNLING_SERVER_URL;
  const keyFile = process.env.CHATTO_RUNLING_API_KEY_FILE;

  if (!serverUrl || !keyFile) {
    throw new Error(
      "Set CHATTO_RUNLING_SERVER_URL and CHATTO_RUNLING_API_KEY_FILE",
    );
  }

  const apiKey = (await readFile(keyFile, "utf8")).trim();
  if (!apiKey) throw new Error("The bot API key file is empty");

  return { serverUrl, apiKey };
}

/** Build the reply workflow. Injectable I/O allows tests without a live server. */
export function createReplyWorkflow(
  loadConfig: () => Promise<BotConfig> = readEnvironment,
  request: typeof fetch = globalThis.fetch,
  answer: typeof generateReply = generateReply,
) {
  return task(
    {
      name: "Reply to Chatto",
      input: webhookInput,
      output: Type.Object({
        deliveryId: Type.String(),
        status: Type.Union([Type.Literal("replied"), Type.Literal("skipped")]),
        replyId: Type.Optional(Type.String()),
      }),
    },
    async (r, input) => {
      const { serverUrl, apiKey } = await loadConfig();
      const base = new URL(serverUrl);

      if (
        !["http:", "https:"].includes(base.protocol) ||
        base.username ||
        base.password
      ) {
        throw new Error(
          "Use an HTTP or HTTPS Chatto server URL without credentials",
        );
      }

      // The destination comes from operator configuration, never from the webhook.
      async function rpc<T>(method: string, body: object): Promise<T> {
        let response: Response;

        try {
          response = await request(
            new URL(`/api/connect/chatto.api.v1.${method}`, base),
            {
              method: "POST",
              headers: {
                "Content-Type": "application/json",
                "Connect-Protocol-Version": "1",
                Authorization: `Bearer ${apiKey}`,
              },
              body: JSON.stringify(body),
              redirect: "error",
              signal: AbortSignal.timeout(10_000),
            },
          );
        } catch {
          throw new Error("Chatto API request did not complete");
        }

        if (!response.ok) {
          throw new Error(`Chatto API returned HTTP ${response.status}`);
        }

        return response.json() as Promise<T>;
      }

      // Confirm the configured credentials belong to the intended bot.
      const viewer = await r.step("Check bot identity", () =>
        rpc<{ user?: { profile?: { id?: string } } }>(
          "ViewerService/GetViewer",
          {},
        ),
      );
      if (viewer.user?.profile?.id !== input.bot_id) {
        throw new Error("The webhook bot does not match the API key");
      }

      // Ignore bot authors to prevent automatic reply loops.
      if (input.message.author_id === input.bot_id) {
        return { deliveryId: input.id, status: "skipped" as const };
      }

      const author = await r.step("Check message author", () =>
        rpc<{ user?: { user?: { isBot?: boolean } } }>("UserService/GetUser", {
          userId: input.message.author_id,
        }),
      );
      if (!author.user?.user) {
        throw new Error("The message author is unavailable");
      }

      if (author.user.user.isBot) {
        return { deliveryId: input.id, status: "skipped" as const };
      }

      // Initial pages contain the root plus the newest replies. Cursor pages
      // contain older replies only, so keep the root ahead of the paged history.
      async function readThread() {
        type Message = { actorId?: string; body?: string };
        type Event = { id?: string; messagePosted?: { message?: Message } };

        const rootId = input.thread_root_id ?? input.message.id;
        let root: Event | undefined;
        let replies: Event[] = [];
        let before: string | undefined;
        const cursors = new Set<string>();

        do {
          const { page } = await rpc<{
            page?: {
              events?: Event[];
              hasOlder?: boolean;
              startCursor?: string;
            };
          }>("ThreadService/GetThreadEvents", {
            roomId: input.room_id,
            threadRootEventId: rootId,
            limit: 100,
            ...(before ? { before } : {}),
          });
          if (!page) throw new Error("Chatto did not return the thread page");

          const events = page.events ?? [];
          root ??= events.find((event) => event.id === rootId);
          replies = [
            ...events.filter((event) => event.id !== rootId),
            ...replies,
          ];

          if (!page.hasOlder) break;

          before = page.startCursor;
          if (!before || cursors.has(before)) {
            throw new Error("Thread pagination did not advance");
          }
          cursors.add(before);
        } while (true);

        // Pages can overlap. Include each message once, in conversation order.
        const seen = new Set<string>();
        return [...(root ? [root] : []), ...replies].flatMap((event) => {
          if (!event.id || seen.has(event.id)) return [];
          seen.add(event.id);

          const message = event.messagePosted?.message;
          if (!message?.body) return [];

          return [
            {
              role:
                message.actorId === input.bot_id
                  ? ("bot" as const)
                  : ("human" as const),
              body: message.body,
            },
          ];
        });
      }

      // Start typing before loading context, and keep it active during composition.
      const stopTyping = await r.step("Start typing", () =>
        startTyping(() =>
          rpc("RoomService/UpdateTypingIndicator", {
            roomId: input.room_id,
            threadRootEventId: input.thread_root_id ?? input.message.id,
          }),
        ),
      );

      // Both the agent and the error fallback use this single reply attempt.
      const sender = createReplySender((text, stepName) =>
        r.step(stepName, async () => {
          const result = await rpc<{ message?: { id?: string } }>(
            "MessageService/CreateMessage",
            {
              roomId: input.room_id,
              body: text,
              threadRootEventId: input.thread_root_id ?? input.message.id,
              inReplyTo: input.message.id,
            },
          );

          const id = result.message?.id;
          if (!id) throw new Error("Chatto did not return a reply ID");

          stopTyping();
          return id;
        }),
      );

      try {
        const thread = await r.step("Load complete thread", readThread);

        await r.step("Compose reply", () =>
          answer(r, {
            message: input.message.body,
            thread,
            readThread,
            sender,
          }),
        );

        if (!sender.id) {
          throw new Error("The agent did not send a chat reply");
        }
      } catch (error) {
        // Notify the user if possible, but keep the run marked as failed.
        await sender.notifyFailure();
        throw error;
      } finally {
        stopTyping();
      }

      return {
        deliveryId: input.id,
        status: "replied" as const,
        replyId: sender.id,
      };
    },
  );
}

export default createReplyWorkflow();
