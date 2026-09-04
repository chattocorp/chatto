/** Durable recovery state that is safe to store without encryption. */
export interface TestBotState {
    resumeCursor?: string;
    processedEventIds: string[];
}
/** Read and validate the bot recovery state. A missing file starts fresh. */
export declare function loadTestBotState(stateFile: string): Promise<TestBotState>;
/** Atomically retain a cursor and a bounded event-ID deduplication window. */
export declare function saveTestBotState(stateFile: string, state: TestBotState): Promise<void>;
/** Add an event ID once and keep only the newest bounded window. */
export declare function rememberProcessedEvent(state: TestBotState, eventId: string): boolean;
//# sourceMappingURL=state.d.ts.map