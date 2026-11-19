/** Mask exact environment values across decoded Docker output chunks, with bounded storage. */
export class OutputMask {
  private pending = '';
  private readonly values: string[];
  constructor(
    values: string[],
    private readonly output: (text: string) => void
  ) {
    this.values = [...new Set(values.filter(Boolean))].sort((a, b) => b.length - a.length);
  }
  write(text: string): void {
    this.pending += text;
    this.flush(false);
  }
  end(): void {
    this.flush(true);
  }

  private flush(final: boolean): void {
    let output = '',
      index = 0;
    while (index < this.pending.length) {
      const tail = this.pending.slice(index);
      // A longer value can share a prefix with a complete shorter value. Wait until the next
      // chunk decides which value is present instead of exposing the longer value's suffix.
      if (
        !final &&
        this.values.some((value) => value.length > tail.length && value.startsWith(tail))
      )
        break;
      const match = this.values.find((value) => this.pending.startsWith(value, index));
      if (match) {
        output += '[REDACTED]';
        index += match.length;
        continue;
      }
      if (final && this.values.some((value) => value.startsWith(tail))) {
        output += '[REDACTED]';
        index = this.pending.length;
        break;
      }
      output += this.pending[index++];
    }
    this.pending = this.pending.slice(index);
    if (output) this.output(output);
  }
}
