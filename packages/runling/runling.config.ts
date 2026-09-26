import chattoAgentDemo from './examples/chatto-agent-demo.ts';
import channelDemo from './examples/channel-demo.ts';
import { defineWebConfig, startWorkflow } from 'runling/web';
import chattoInputDemo from './examples/chatto-input-demo.ts';
import chattoCoordinatorDemo from './examples/chatto-coordinator-demo.ts';
import chattoPlanDemo from './examples/chatto-plan-demo.ts';
import joke from './examples/joke.ts';
import makePullRequest from './examples/make-pr.ts';

export default defineWebConfig({
  webhooks: {
    'chatto-agent-demo': chattoAgentDemo.route,
    'channel-demo': startWorkflow(channelDemo),
    'chatto-input-demo': chattoInputDemo.route,
    'chatto-plan-demo': chattoPlanDemo.route,
    'chatto-coordinator-demo': chattoCoordinatorDemo.route,
    joke: startWorkflow(joke),
    'make-pr': startWorkflow(makePullRequest)
  }
});
