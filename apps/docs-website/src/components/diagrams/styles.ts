/** Shared SVG styles for architecture diagrams. Dark mode is the default. */
export const baseStyles = `
    text { font-family: system-ui, -apple-system, sans-serif; }
    .label { font-size: 14px; fill: #e0e0e0; font-weight: 600; }
    .sublabel { font-size: 11px; fill: #888; }
    .port { font-size: 10px; fill: #666; font-family: monospace; }
    .proto { font-size: 10px; fill: #777; font-family: monospace; }
    .box { rx: 10; ry: 10; stroke-width: 1.5; }
    .conn { stroke: #444; stroke-width: 1.5; fill: none; }
    .conn-dash { stroke: #444; stroke-width: 1.5; fill: none; stroke-dasharray: 6 4; }
    .conn-repl { stroke: #8a5a8a; stroke-width: 1.5; fill: none; stroke-dasharray: 5 3; }
    .region { rx: 14; ry: 14; fill: none; stroke: #2a2a2a; stroke-width: 1; }
    .region-label { font-size: 10px; fill: #555; font-weight: 600; letter-spacing: 0.1em; }

    .dot { fill: #38bdf8; }
    .dot-yellow { fill: #c8b44a; }
    .dot-blue { fill: #7dd3fc; }
    .dot-repl { fill: #b88ad0; }

    .box-browser { fill: #1e1e1e; stroke: #555; }
    .box-caddy { fill: #162016; stroke: #3a7a3a; }
    .box-chatto { fill: #161620; stroke: #5a5ab8; }
    .box-nats { fill: #201620; stroke: #8a5a8a; }
    .box-nats-leader { fill: #281828; stroke: #b070b0; stroke-width: 2; }
    .box-livekit { fill: #162020; stroke: #3a8a8a; }

    @media (prefers-color-scheme: light) {
      .label { fill: #1a1a1a; }
      .sublabel { fill: #666; }
      .port { fill: #888; }
      .proto { fill: #888; }
      .conn, .conn-dash { stroke: #ccc; }
      .conn-repl { stroke: #c090c0; }
      .region { stroke: #ddd; }
      .region-label { fill: #aaa; }
      .box-browser { fill: #f8f8f8; stroke: #bbb; }
      .box-caddy { fill: #eef6ee; stroke: #5aaa5a; }
      .box-chatto { fill: #eeeef6; stroke: #7a7ad0; }
      .box-nats { fill: #f6eef6; stroke: #aa7aaa; }
      .box-nats-leader { fill: #f0e4f0; stroke: #c080c0; stroke-width: 2; }
      .box-livekit { fill: #eef6f6; stroke: #5aaaaa; }
    }
`;

/** Standard easing for unidirectional animated dots. */
export const EASE = '0.4 0 0.2 1';

/** Standard easing for bidirectional (bounce) dots — needs two segments. */
export const EASE_BOUNCE = '0.4 0 0.2 1;0.4 0 0.2 1';
