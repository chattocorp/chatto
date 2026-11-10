import { expect, test } from 'vitest';
import { OutputMask } from './docker-output.ts';

test('masks secrets at every possible chunk boundary, including shared prefixes', () => {
  const text = 'start secret-token; secret; secret-token end';
  for (let boundary = 0; boundary <= text.length; boundary++) {
    const output: string[] = [];
    const mask = new OutputMask(['secret', 'secret-token', ''], (text) => output.push(text));
    mask.write(text.slice(0, boundary));
    mask.write(text.slice(boundary));
    mask.end();
    expect(output.join('')).toBe('start [REDACTED]; [REDACTED]; [REDACTED] end');
  }
});

test('streams ordinary text immediately while retaining only a possible secret prefix', () => {
  const output: string[] = [];
  const mask = new OutputMask(['private-token'], (text) => output.push(text));
  mask.write('live output\npriva');
  expect(output.join('')).toBe('live output\n');
  mask.write('te-token\n');
  expect(output.join('')).toBe('live output\n[REDACTED]\n');
  mask.end();
});

test('flushes an incomplete secret safely when output ends', () => {
  const output: string[] = [];
  const mask = new OutputMask(['private-token'], (text) => output.push(text));
  mask.write('output priva');
  mask.end();
  expect(output.join('')).toBe('output [REDACTED]');
});
