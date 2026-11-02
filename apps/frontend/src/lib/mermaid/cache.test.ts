import { expect, it } from 'vitest';
import { getMermaidLayout, rememberMermaidLayout } from './cache';
import { MAX_MERMAID_PREVIEWS } from './policy';

it('bounds layouts per owner and does not share private results between owners', () => {
  const owner = {};
  for (let index = 0; index <= MAX_MERMAID_PREVIEWS; index++) {
    rememberMermaidLayout(owner, `source ${index}`, `<svg>${index}</svg>`);
  }
  expect(getMermaidLayout(owner, 'source 0')).toBeUndefined();
  expect(getMermaidLayout(owner, 'source 1')).toBe('<svg>1</svg>');
  expect(getMermaidLayout({}, 'source 1')).toBeUndefined();
  expect(getMermaidLayout(owner, 'changed source')).toBeUndefined();
});
