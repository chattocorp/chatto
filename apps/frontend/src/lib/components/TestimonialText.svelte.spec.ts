import { describe, expect, it } from 'vitest';
import { render } from 'vitest-browser-svelte';
import TestimonialText from './TestimonialText.svelte';

describe('TestimonialText', () => {
  it('renders paragraphs and restrained inline Markdown', () => {
    const { container } = render(TestimonialText, {
      props: { testimonial: 'First **thought**.\n\nSecond *thought* with `care`.' }
    });

    expect(container.querySelectorAll('p')).toHaveLength(2);
    expect(container.querySelector('strong')?.textContent).toBe('thought');
    expect(container.querySelector('em')?.textContent).toBe('thought');
    expect(container.querySelector('code')?.textContent).toBe('care');
  });

  it('does not enable links, headings, lists, tables, images, or source HTML', () => {
    const testimonial = [
      '# Heading',
      '',
      '- item',
      '',
      '[link](https://example.com) ![image](https://example.com/image.png)',
      '',
      '| table |',
      '| --- |',
      '',
      '<strong>source HTML</strong>'
    ].join('\n');
    const { container } = render(TestimonialText, { props: { testimonial } });

    expect(container.querySelector('a, h1, ul, ol, table, img')).toBeNull();
    expect(container.textContent).toContain('# Heading');
    expect(container.textContent).toContain('[link](https://example.com)');
    expect(container.textContent).toContain('<strong>source HTML</strong>');
  });
});
