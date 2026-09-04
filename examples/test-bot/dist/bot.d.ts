/** Runtime configuration for the public-API example bot. */
export interface TestBotConfig {
    serverUrl: string;
    apiKeyFile: string;
    stateFile: string;
}
/** Run until the process receives a shutdown signal or the server forbids reconnect. */
export declare function runTestBot(config: TestBotConfig, signal: AbortSignal): Promise<void>;
//# sourceMappingURL=bot.d.ts.map