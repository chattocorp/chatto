// Set telemetry policy before loading Turbo's CLI, without shell-specific syntax.
// Turbo retains argument parsing, platform selection, signal handling, and exit codes.
process.env.TURBO_TELEMETRY_DISABLED = "1";
await import("turbo");
