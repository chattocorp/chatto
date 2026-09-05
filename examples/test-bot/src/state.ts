import { randomUUID } from "node:crypto";
import { mkdir, readFile, rename, rm, writeFile } from "node:fs/promises";
import path from "node:path";

const MAX_PROCESSED_EVENT_IDS = 2_048;

/** Durable recovery state that is safe to store without encryption. */
export interface TestBotState {
  resumeCursor?: string;
  processedEventIds: string[];
}

/** Persist one immutable snapshot of the bot recovery state. */
export type TestBotStateSaver = (state: TestBotState) => Promise<void>;

/** Read and validate the bot recovery state. A missing file starts fresh. */
export async function loadTestBotState(
  stateFile: string,
): Promise<TestBotState> {
  let raw: string;
  try {
    raw = await readFile(stateFile, "utf8");
  } catch (error) {
    if ((error as NodeJS.ErrnoException).code === "ENOENT") {
      return { processedEventIds: [] };
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
  return {
    ...(candidate.resumeCursor ? { resumeCursor: candidate.resumeCursor } : {}),
    processedEventIds: candidate.processedEventIds.slice(
      -MAX_PROCESSED_EVENT_IDS,
    ),
  };
}

/** Atomically retain the cursor and bounded event-ID deduplication window. */
export async function saveTestBotState(
  stateFile: string,
  state: TestBotState,
): Promise<void> {
  const snapshot: TestBotState = {
    ...(state.resumeCursor ? { resumeCursor: state.resumeCursor } : {}),
    processedEventIds: state.processedEventIds.slice(-MAX_PROCESSED_EVENT_IDS),
  };
  const serialized = `${JSON.stringify(snapshot)}\n`;
  const directory = path.dirname(stateFile);
  await mkdir(directory, { recursive: true, mode: 0o700 });
  const temporaryFile = `${stateFile}.${process.pid}.${randomUUID()}.tmp`;
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

/** Serialize state-file replacement while concurrent reply jobs mutate state. */
export function serialTestBotStateSaver(stateFile: string): TestBotStateSaver {
  let tail = Promise.resolve();
  return (state) => {
    const snapshot: TestBotState = {
      ...(state.resumeCursor ? { resumeCursor: state.resumeCursor } : {}),
      processedEventIds: [...state.processedEventIds],
    };
    const operation = tail.then(() => saveTestBotState(stateFile, snapshot));
    tail = operation.catch(() => undefined);
    return operation;
  };
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
