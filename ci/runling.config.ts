import { defineWebConfig, startWorkflow } from 'runling/web';
import tests from './workflows/tests.ts';

export default defineWebConfig({ webhooks: { tests: startWorkflow(tests) } });
