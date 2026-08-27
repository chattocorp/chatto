import { describe, expect, it } from 'vitest';
import { highlightTreeSpans } from './codeFenceHighlighting';

describe('highlightTreeSpans', () => {
  it('flattens nested Lowlight tokens without losing source offsets', () => {
    expect(
      highlightTreeSpans([
        { type: 'text', value: 'const ' },
        {
          type: 'element',
          properties: { className: ['hljs-title'] },
          children: [
            { type: 'text', value: 'answer' },
            {
              type: 'element',
              properties: { className: ['hljs-built_in'] },
              children: [{ type: 'text', value: '()' }]
            }
          ]
        },
        { type: 'text', value: ';' }
      ])
    ).toEqual([
      { from: 6, to: 12, classes: 'hljs-title' },
      { from: 12, to: 14, classes: 'hljs-title hljs-built_in' }
    ]);
  });
});
