import channelDemo from './examples/channel-demo.ts';
import { defineWebConfig, startWorkflow } from 'runling/web';
import joke from './examples/joke.ts';
import makePullRequest from './examples/make-pr.ts';
import docker from './examples/docker.ts';

export default defineWebConfig({
  webhooks: {
    docker: startWorkflow(docker),
    'channel-demo': startWorkflow(channelDemo),
    joke: startWorkflow(joke),
    'make-pr': startWorkflow(makePullRequest)
  }
});
