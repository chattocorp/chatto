import { describe, expect, it, vi } from 'vitest';
import { sanitizeSvg } from './sanitizeSvg';

const wrap = (content: string) =>
  `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">${content}</svg>`;

describe('SVG preview sanitization', () => {
  it('keeps shapes, text, transparency and local gradients as a decodable SVG', async () => {
    const clean = sanitizeSvg(
      wrap(
        '<defs><linearGradient id="paint"><stop offset="0" stop-color="red"/><stop offset="1" stop-color="red"/></linearGradient></defs>' +
          '<rect width="10" height="10" fill="url(#paint)"/><text x="1" y="19">Hello</text>'
      )
    );
    expect(clean).toContain('linearGradient');
    expect(clean).toContain('Hello');
    const image = new Image();
    image.src = `data:image/svg+xml,${encodeURIComponent(clean)}`;
    await image.decode();
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 20;
    const context = canvas.getContext('2d')!;
    context.drawImage(image, 0, 0);
    expect([...context.getImageData(5, 5, 1, 1).data]).toEqual([255, 0, 0, 255]);
    expect([...context.getImageData(15, 5, 1, 1).data]).toEqual([0, 0, 0, 0]);
  });

  it.each([
    '<script>alert(1)</script>',
    '<foreignObject><div xmlns="http://www.w3.org/1999/xhtml" onload="alert(1)">HTML</div></foreignObject>',
    '<a href="javascript:alert(1)"><rect width="10" height="10"/></a>',
    '<animate attributeName="href" values="javascript:alert(1)" onbegin="alert(1)"/>',
    '<image href="https://example.invalid/tracker.svg"/>',
    '<style>@import "https://example.invalid/style.css";</style>',
    '<filter id="blur"><feGaussianBlur stdDeviation="999999"/></filter>',
    '<pattern id="tile"><rect width="1" height="1"/></pattern>'
  ])('removes active or expensive markup: %s', (payload) => {
    const clean = sanitizeSvg(
      wrap(payload + '<rect width="10" height="10" onload="alert(1)" style="fill:red"/>')
    );
    const root = new DOMParser().parseFromString(clean, 'image/svg+xml');
    expect(
      root.querySelector('script, foreignObject, a, animate, image, style, filter, pattern')
    ).toBeNull();
    expect(root.querySelector('[onload], [onbegin], [style], [href]')).toBeNull();
    expect(clean).not.toContain('alert(');
    expect(clean).not.toContain('example.invalid');
    expect(root.querySelector('rect')).not.toBeNull();
  });

  it('removes both forms of use references before the browser can expand them', () => {
    const source =
      '<svg xmlns="http://www.w3.org/2000/svg" xmlns:xlink="http://www.w3.org/1999/xlink">' +
      '<defs><g id="a"><use href="#a"/><use xlink:href="#a"/></g></defs><use href="#a"/></svg>';
    const clean = sanitizeSvg(source);
    expect(clean).not.toContain('<use');
    expect(clean).not.toContain('href');
  });

  it('removes external paint references and event attributes regardless of case', () => {
    const clean = sanitizeSvg(
      wrap(
        '<rect ONLOAD="alert(1)" fill="url(https://example.invalid/a.svg#paint)" stroke="url(//example.invalid/a)"/>'
      )
    );
    expect(clean).not.toMatch(/onload|alert|example\.invalid/i);
  });

  it.each([
    'url(https://example.invalid/a)',
    'u\\72l(https://example.invalid/a)',
    'url/**/(https://example.invalid/a)'
  ])('does not allow CSS syntax to disguise an external paint reference: %s', (paint) => {
    const clean = sanitizeSvg(wrap(`<rect fill="${paint}"/>`));
    expect(clean).not.toContain('example.invalid');
  });

  it.each([
    '<!DOCTYPE svg [<!ENTITY a "expanded">]>' + wrap('<text>&a;</text>'),
    '<!DOCTYPE svg SYSTEM "https://example.invalid/evil.dtd">' + wrap(''),
    wrap('x'.repeat(100_001))
  ])('rejects declarations and excessive source before parsing', (source) => {
    const parse = vi.spyOn(DOMParser.prototype, 'parseFromString');
    try {
      expect(() => sanitizeSvg(source)).toThrow('preview policy');
      expect(parse).not.toHaveBeenCalled();
    } finally {
      parse.mockRestore();
    }
  });

  it.each([
    '<svg xmlns="http://www.w3.org/2000/svg"><g></svg>',
    '<html xmlns="http://www.w3.org/1999/xhtml"/>',
    '<svg/>',
    ''
  ])('rejects malformed documents and non-SVG roots', (source) => {
    expect(() => sanitizeSvg(source)).toThrow('Invalid SVG');
  });

  it('bounds element count and nesting before sanitizing', () => {
    expect(() => sanitizeSvg(wrap('<rect/>'.repeat(1_000)))).toThrow('structure');
    expect(() => sanitizeSvg(wrap('<g>'.repeat(32) + '</g>'.repeat(32)))).toThrow('structure');
    expect(() => sanitizeSvg(wrap('<rect/>'.repeat(999)))).not.toThrow();
    expect(() => sanitizeSvg(wrap('<g>'.repeat(31) + '</g>'.repeat(31)))).not.toThrow();
  });
});
