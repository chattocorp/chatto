import { defineWebConfig } from 'runling/web';
import { chattoSource } from './chatto/source.ts';

export default defineWebConfig({
  sources: { chatto: chattoSource }
});
