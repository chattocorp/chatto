import { randomUUID } from "node:crypto";
import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";

const MAX_PROCESSED_EVENT_IDS = 2_048;
const MAX_PENDING_REPLIES = 64;

/** A placeholder reply that still needs its final AI-generated body. */
export interface PendingReply {
  sourceEventId: string;
  replyEventId: string;
}

/** Durable recovery state that is safe to store without encryption. */
export interface TestBotState {
  resumeCursor?: string;
  processedEventIds: string[];
  pendingReplies: PendingReply[];
}

/** Read and validate the bot recovery state. A missing file starts fresh. */
export async function loadTestBotState(
  stateFile: string,
): Promise<TestBotState> {
  let raw: string;
  try {
    raw = await readFile(stateFile, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return { processedEventIds: [], pendingReplies: [] };
    }
    throw error;
  }

  const parsed: unknown = JSON.parse(raw);
  if (!parsed || typeof parsed !== "object") {
    throw new Error("test bot state must be an object");
  }
  const candidate = parsed as {
    resumeCursor?: unknown;
    processedEventIds?: unknown;
    pendingReplies?: unknown;
  };
  if (
    candidate.resumeCursor !== undefined &&
    typeof candidate.resumeCursor !== "string"
  ) {
    throw new Error("test bot resume cursor must be a string");
  }
  if (
    !Array.isArray(candidate.processedEventIds) ||
    !candidate.processedEventIds.every((id) => typeof id === "string")
  ) {
    throw new Error("test bot processed event IDs must be strings");
  }
  if (
    candidate.pendingReplies !== undefined &&
    (!Array.isArray(candidate.pendingReplies) ||
      !candidate.pendingReplies.every(isPendingReply))
  ) {
    throw new Error("test bot pending replies must contain event IDs");
  }
  return {
    ...(candidate.resumeCursor ? { resumeCursor: candidate.resumeCursor } : {}),
    processedEventIds: candidate.processedEventIds.slice(
      -MAX_PROCESSED_EVENT_IDS,
    ),
    pendingReplies: (candidate.pendingReplies ?? []).slice(
      -MAX_PENDING_REPLIES,
    ),
  };
}

function isPendingReply(value: unknown): value is PendingReply {
  if (!value || typeof value !== "object") return false;
  const candidate = value as Partial<PendingReply>;
  return (
    typeof candidate.sourceEventId === "string" &&
    candidate.sourceEventId.length > 0 &&
    typeof candidate.replyEventId === "string" &&
    candidate.replyEventId.length > 0
  );
}

/** Atomically retain the cursor, deduplication window, and pending replies. */
export async function saveTestBotState(
  stateFile: string,
  state: TestBotState,
): Promise<void> {
  const directory = path.dirname(stateFile);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const temporaryFile = `${stateFile}.${process.pid}.${randomUUID()}.tmp`;
  const serialized = `${JSON.stringify({
    ...(state.resumeCursor ? { resumeCursor: state.resumeCursor } : {}),
    processedEventIds: state.processedEventIds.slice(-MAX_PROCESSED_EVENT_IDS),
    pendingReplies: state.pendingReplies.slice(-MAX_PENDING_REPLIES),
  })}\n`;
  try {
    await writeFile(temporaryFile, serialized, {
      encoding: "utf8",
      mode: 0o600,
    });
    await rename(temporaryFile, stateFile);
  } finally {
    await rm(temporaryFile, { force: true });
  }
}

/** Add an event ID once and keep only the newest bounded window. */
export function rememberProcessedEvent(
  state: TestBotState,
  eventId: string,
): boolean {
  if (!eventId || state.processedEventIds.includes(eventId)) return false;
  state.processedEventIds.push(eventId);
  if (state.processedEventIds.length > MAX_PROCESSED_EVENT_IDS) {
    state.processedEventIds.splice(
      0,
      state.processedEventIds.length - MAX_PROCESSED_EVENT_IDS,
    );
  }
  return true;
}

/** Find the placeholder reply for a source event, when one exists. */
export function pendingReplyId(
  state: TestBotState,
  sourceEventId: string,
): string | undefined {
  return state.pendingReplies.find(
    (reply) => reply.sourceEventId === sourceEventId,
  )?.replyEventId;
}

/** Remember the placeholder reply for one source event. */
export function rememberPendingReply(
  state: TestBotState,
  sourceEventId: string,
  replyEventId: string,
): void {
  forgetPendingReply(state, sourceEventId);
  state.pendingReplies.push({ sourceEventId, replyEventId });
  if (state.pendingReplies.length > MAX_PENDING_REPLIES) {
    state.pendingReplies.splice(
      0,
      state.pendingReplies.length - MAX_PENDING_REPLIES,
    );
  }
}

/** Forget a placeholder after its source event is fully processed. */
export function forgetPendingReply(
  state: TestBotState,
  sourceEventId: string,
): void {
  const index = state.pendingReplies.findIndex(
    (reply) => reply.sourceEventId === sourceEventId,
  );
  if (index >= 0) state.pendingReplies.splice(index, 1);
}
