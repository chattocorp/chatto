// Set telemetry policy before loading Turbo's CLI, without shell-specific syntax.
// Turbo retains argument parsing, platform selection, signal handling, and exit codes.
process.env.TURBO_TELEMETRY_DISABLED = '1';
// Use only the local cache unless the caller selects cache sources. CI selects
// its remote cache in .github/actions/setup. See ADR-102.
process.env.TURBO_CACHE ??= 'local:rw';
await import('turbo');
