import { defineWebConfig } from 'runling/web';
import { chattoSource, chattoWorkflow } from './chatto/source.ts';

export default defineWebConfig({
  workflows: { chattoThread: chattoWorkflow },
  sources: { chatto: chattoSource }
});
