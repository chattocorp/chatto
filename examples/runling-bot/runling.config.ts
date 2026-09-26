import { defineWebConfig, startWorkflow } from 'runling/web';
import reply from './reply.ts';

export default defineWebConfig({
  webhooks: { chatto: startWorkflow(reply) }
});
