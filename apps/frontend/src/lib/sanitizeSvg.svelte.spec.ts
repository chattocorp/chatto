import { describe, expect, it, vi } from 'vitest';
import { sanitizeSvg } from './sanitizeSvg';

const wrap = (content: string) =>
  `<svg xmlns="http://www.w3.org/2000/svg" width="20" height="20">${content}</svg>`;

describe('SVG preview sanitization', () => {
  it('keeps simple numeric, colour and transform animations', () => {
    const clean = sanitizeSvg(
      wrap(
        '<rect width="20" height="20">' +
          '<animate attributeName="x" values="0;10;0" dur="2s" repeatCount="indefinite"/>' +
          '<animate attributeName="fill" from="red" to="blue" dur="200ms" fill="freeze"/>' +
          '<animateTransform attributeName="transform" type="rotate" from="0" to="360" dur="4s"/>' +
          '</rect>'
      )
    );
    const root = new DOMParser().parseFromString(clean, 'image/svg+xml');
    expect(root.querySelectorAll('animate, animateTransform')).toHaveLength(3);
    expect(root.querySelector('[attributeName="fill"]')?.getAttribute('fill')).toBe('freeze');
  });

  it.each([
    'attributeName="href" values="x;javascript:alert(1)"',
    'attributeName="style" to="fill:red"',
    'attributeName="id" to="other"',
    'attributeName="fill" values="red;url(https://example.invalid/paint)"',
    'attributeName="fill" values="red;var(--paint,url(https://example.invalid/paint))"',
    'attributeName="fill" values="red;blue" from="url(https://example.invalid/paint)"',
    'attributeName="fill" values="red;blue" by="url(https://example.invalid/paint)"',
    'attributeName="x" values="0;1" begin="other.click"',
    'attributeName="x" values="0;999999999"',
    'attributeName="x" values="0;NaN"'
  ])('drops unsafe animation targets, values and timing: %s', (attributes) => {
    const clean = sanitizeSvg(wrap(`<rect><animate ${attributes} dur="1s"/></rect>`));
    expect(
      new DOMParser().parseFromString(clean, 'image/svg+xml').querySelector('animate')
    ).toBeNull();
  });

  it('bounds animation count and removes event handlers and target references', () => {
    const clean = sanitizeSvg(
      wrap(
        '<rect>' +
          '<animate attributeName="x" values="0;1" dur="1s" onbegin="alert(1)" href="#other"/>'.repeat(
            33
          ) +
          '</rect>'
      )
    );
    const root = new DOMParser().parseFromString(clean, 'image/svg+xml');
    expect(root.querySelectorAll('animate')).toHaveLength(32);
    expect(root.querySelector('[onbegin], [href]')).toBeNull();
  });

  it.each([
    ['g', "url('#g')"],
    ['g', 'url("#g")'],
    ['g', 'url( #g )'],
    ['g', 'url( "#g" ) red'],
    ['g', 'url(#g) red'],
    ['title', 'url(#title)'],
    ['images', 'url(#images)'],
    ['body', 'url(#body)'],
    ['links', 'url(#links)'],
    ['forms', 'url(#forms)'],
    ['fonts', 'url(#fonts)']
  ])('preserves local gradient %s with paint %s', async (id, paint) => {
    const clean = sanitizeSvg(
      wrap(
        `<defs><linearGradient id="${id}"><stop stop-color="lime"/></linearGradient></defs>` +
          `<rect width="20" height="20" fill="${paint.replaceAll('"', '&quot;')}"/>`
      )
    );
    const root = new DOMParser().parseFromString(clean, 'image/svg+xml');
    expect(root.querySelector('linearGradient')?.id).toBe(id);
    expect(root.querySelector('rect')?.getAttribute('fill')).toBe(paint);
    const image = new Image();
    image.src = `data:image/svg+xml,${encodeURIComponent(clean)}`;
    await image.decode();
    const canvas = document.createElement('canvas');
    canvas.width = canvas.height = 20;
    const context = canvas.getContext('2d')!;
    context.drawImage(image, 0, 0);
    expect([...context.getImageData(5, 5, 1, 1).data]).toEqual([0, 255, 0, 255]);
  });

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
    'url("https://example.invalid/a") red',
    "url('#g') url(https://example.invalid/a)",
    'url(\'#g") red',
    'u\\72l(https://example.invalid/a)',
    'url/**/(https://example.invalid/a)',
    'var(--paint,url(https://example.invalid/a))',
    'v\\61r(--paint,url(https://example.invalid/a))'
  ])('does not allow CSS syntax to disguise an external paint reference: %s', (paint) => {
    const clean = sanitizeSvg(wrap(`<rect fill="${paint.replaceAll('"', '&quot;')}"/>`));
    expect(clean).not.toContain('example.invalid');
    expect(
      new DOMParser()
        .parseFromString(clean, 'image/svg+xml')
        .querySelector('rect')
        ?.hasAttribute('fill')
    ).toBe(false);
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
