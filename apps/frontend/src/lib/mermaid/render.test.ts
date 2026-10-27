import { describe, expect, it } from 'vitest';
import { renderMermaid } from './render';

describe('Mermaid image rendering', () => {
  it('frames nested default groups while preserving author-coloured groups', () => {
    const source = 'flowchart TB\nsubgraph Outer\nsubgraph Inner\nA-->B\nend\nend';
    const svg = renderMermaid(source);
    expect(svg.match(/class="chatto-pele-frame"/g)).toHaveLength(2);
    expect(svg.match(/class="chatto-pele-inset"/g)).toHaveLength(2);
    expect(svg).toContain('>Inner</text>');
    expect(svg).toContain('>Outer</text>');
    const styled = renderMermaid(source + '\nstyle Inner fill:#123456');
    expect(styled).toContain('#123456');
    expect(styled.match(/class="chatto-pele-frame"/g)).toHaveLength(1);
  });
  it.each([
    'flowchart LR\nA[Hello] --> B[World]',
    'sequenceDiagram\nAlice->>Bob: Hello',
    'classDiagram\nAnimal <|-- Duck',
    'stateDiagram-v2\n[*] --> Ready\nReady --> [*]',
    'erDiagram\nCUSTOMER ||--o{ ORDER : places',
    'gitGraph\ncommit\nbranch feature\ncheckout feature\ncommit'
  ])('renders supported diagram syntax: %s', (source) => {
    expect(renderMermaid(source)).toContain('<svg');
  });

  it.each([
    '---\nconfig:\n  securityLevel: loose\n---\nflowchart LR\nA-->B',
    '%%{init: {"securityLevel":"loose"}}%%\nflowchart LR\nA-->B',
    'flowchart LR\n%%{init: {}}%%\nA-->B',
    'x'.repeat(10_001),
    'not a diagram',
    'flowchart LR\n' + Array.from({ length: 251 }, (_, i) => `A-->B${i}`).join('\n'),
    'flowchart LR\n' + Array.from({ length: 600 }, (_, i) => `A${i}`).join('\n')
  ])('rejects configuration, invalid syntax or excessive complexity (%#)', (source) => {
    expect(() => renderMermaid(source)).toThrow();
  });

  it('does not emit links, scripts or HTML labels', () => {
    const svg = renderMermaid(
      'flowchart LR\nA["<script>alert(1)</script>"] --> B\nclick B "https://example.invalid/track"'
    );
    expect(svg).not.toMatch(/<(?:a|script|foreignObject)\b|\son\w+=/u);
    expect(svg).not.toContain('https://example.invalid/track');
  });

  it('omits external image shapes', () => {
    const svg = renderMermaid(
      'flowchart LR\nA@{ img: "https://example.invalid/track.svg", label: "Image", h: 60, w: 60 }'
    );
    expect(svg).not.toContain('<image');
    expect(svg).not.toContain('https://example.invalid');
  });
});
