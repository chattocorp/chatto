import { realpath } from "node:fs/promises";
import { execa } from "execa";
import { task } from "runling";

/** Update a dedicated, clean main checkout without discarding local work. */
export const refreshCheckout = task(async function refreshChattoCheckout(ctx, directory: string) {
  const git = (...args: string[]) => execa("git", args, {
    cwd: directory,
    cancelSignal: ctx.signal,
    timeout: 60_000,
    env: { GIT_TERMINAL_PROMPT: "0" },
  });
  const root = (await git("rev-parse", "--show-toplevel")).stdout;
  if (await realpath(root) !== await realpath(directory)) throw new Error("CHATTO_WORKING_COPY must be the repository root");
  if ((await git("branch", "--show-current")).stdout !== "main") throw new Error("Chatto working copy must be on main");
  if ((await git("status", "--porcelain")).stdout) throw new Error("Chatto working copy must be clean");
  await git("fetch", "origin", "main");
  const revision = (await git("rev-parse", "FETCH_HEAD")).stdout;
  // Refuse ahead/diverged local history as well as conflicts. Never reset or stash.
  await git("merge-base", "--is-ancestor", "HEAD", revision);
  await git("-c", "core.hooksPath=/dev/null", "merge", "--ff-only", "--no-edit", revision);
  ctx.signal.throwIfAborted();
  return revision;
});
