# Chatto GPUix

Read README.md for the scope, commands, and dependency boundary.

- This is a native Svelte renderer, with no DOM or browser globals.
- Use gpuix-svelte's bundled Svelte build and the gpuixHTML type namespace.
  Keep the custom build nested in gpuix-svelte. The root workspace supplies
  @chatto/client; do not add a separate registry Svelte dependency to this app.
- Connections, credentials, and realtime state use @chatto/client. Keep host
  settings and secure password UI here. Do not introduce automatic external
  connections through Markdown or remote assets.
- Keep the runtime independent of the Rust native experiment. A connected
  version uses @chatto/client rather than another JS protocol.
- Run mise check, mise test, and mise build after behavior changes. Native
  headless tests require macOS or Windows. Run checks locally. Do not add CI
  jobs for this experiment.
