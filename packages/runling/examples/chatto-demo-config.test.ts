import { expect, test, vi } from "vitest";
import { createWorkflowContext } from "runling";
import { createChattoPlanDemo } from "./chatto-plan-demo.ts";
import { createChattoCoordinatorDemo } from "./chatto-coordinator-demo.ts";

for (const directory of ["", "   "]) {
  test(`checkout demos reject an unconfigured directory ${JSON.stringify(directory)} before any work`, async () => {
    const post = vi.fn(async () => {});
    const unexpectedWork = vi.fn(async (): Promise<never> => {
      throw new Error("Must not access a checkout or create an agent");
    });
    const demos = [
      createChattoPlanDemo({
        directory, post, refresh: unexpectedWork, createPlanner: unexpectedWork,
      }),
      createChattoCoordinatorDemo({ directory, post, createAgent: unexpectedWork }),
    ];
    for (const demo of demos) {
      await expect(demo(createWorkflowContext(), {
        version: 1,
        id: "delivery",
        type: "message.created",
        triggers: ["direct_message"],
        occurred_at: "now",
        bot_id: "bot",
        room_id: "dm",
        thread_root_id: null,
        message: { id: "root", body: "Plan a change", author_id: "user" },
      })).rejects.toThrow("Set CHATTO_WORKING_COPY");
    }
    expect(unexpectedWork).not.toHaveBeenCalled();
    expect(post).not.toHaveBeenCalled();
  });
}
