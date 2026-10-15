# ChattoBot

ChattoBot is the Chatto project's assistant. It runs in a Chatto server as a bot
account. It answers questions about Chatto from the documentation, and
maintainers can ask it to create and update GitHub issues.

ChattoBot is a [Runling](../runling/README.md) application. Each thread in which
someone addresses the bot gets one conversation run with one agent session. The
Runling console shows each run, its turns, and its tool calls.

## How it works

1. The `chatto` event source connects to Chatto as the bot. It receives the
   messages that address the bot: direct messages, mentions, and replies to
   the bot.
2. The router sends each message to the conversation run of its thread. When a
   thread has no run, the router starts one.
3. The conversation answers the messages in turns. A turn includes all messages
   that arrived since the previous turn, and the other new messages in the
   thread as context. The first turn includes the thread so far.
4. The agent's final text is posted to the thread as the reply.

A conversation ends after one hour without messages to the bot. The next
message starts a new conversation, which reads the thread again. A restart of
the bot also ends all conversations.

## Tools

| Tool             | What it does                                                          |
| ---------------- | --------------------------------------------------------------------- |
| `fetchPage`      | Reads the Chatto documentation and the Awesome Chatto list.           |
| `readThread`     | Reads the newest messages of the thread.                              |
| `viewAttachment` | Looks at an image or a small text file in the thread.                 |
| `webSearch`      | Searches the web with Tavily, when configured.                        |
| `browsePage`     | Reads a web page with Cloudflare Browser Run, when configured.        |
| `gh`             | Runs read-only GitHub CLI commands, when GitHub is configured.        |
| `githubWork`     | Starts a GitHub worker for a request from a maintainer.               |
| `investigate`    | Starts an investigator subagent for the source code, when configured. |

Everyone who can address the bot can use the read tools. GitHub changes are
reserved for maintainers:

- **Changes need a maintainer request.** The bot starts a GitHub worker only
  for an addressed message from a user in `CHATTO_MAINTAINER_USER_IDS` in the
  current turn. The host checks the authenticated author. Claims in message
  text and old maintainer messages do not grant access. Without configured
  maintainers, the bot offers no GitHub worker.
- **The page reader follows links only.** `browsePage` reads only pages from
  search results or links that people posted, so the bot cannot send thread
  content to an address that it put together.
- **Reads stay in the thread.** Read tools return their results to the bot,
  which answers in the same thread.

## Background jobs

Long work runs in the background, like subagents in an agent harness: an
investigation or GitHub work. The tool returns at once,
and the bot keeps talking. Its turns list the running jobs with their latest
step, so it can answer status questions. When a job finishes, its result
reaches the bot as a new turn, and the bot replies to the person who asked.

The steps of a job are not posted to the thread yet. They will show as the bot's
temporary status in the thread when Chatto supports it (#2787).

## GitHub work

A maintainer can ask the bot to create an issue, update its text or labels, or
add a comment. The conversation starts `tasks/github.ts` as a Runling child task
through `githubWork`. The worker uses the same model and reasoning effort as
the conversation. It receives the original request and context from the
conversation, uses one `gh` tool, and returns its result to the conversation.
The conversation posts the reply.

When a request does not identify work to do, the worker asks for clarification
before using tools. For a clear request, it can read GitHub to resolve details.
The CLI runs the agent task directly and shows one GitHub worker. Command steps
show names and numeric issue or PR targets, such as `gh issue edit #12`;
titles, bodies, search terms, and API paths are omitted from those labels.

Each message can start one worker, and one GitHub worker can run at a time in
a thread. A later request starts a new session. Session files are stored in
`.runling/subagents/github/`.

The worker can read GitHub and make changes allowed by an issue-write token
for the configured repository. It has no shell or file tools. The CLI runner
blocks commands that access local files, credentials, editors, or other hosts.

This first version checks who made the request. The agents decide which
commands fulfill it. There is no command authorization classifier and no
separate reaction approval for each change. The maintainer gate does not prove
that every command matches the request.

You can also run the task directly from `packages/chattobot/`:

```sh
pnpm exec runling run tasks/github.ts "List the five newest open issues"
```

Runling loads `.env` in the current directory. Set `CHATTO_AGENT_MODEL`, the
model provider credentials, and the three `CHATTO_GITHUB_*` settings listed
below. The task uses the configured GitHub App credentials, not your `gh auth`
login. Local execution does not need `CHATTO_URL`, `CHATTO_API_KEY`, or
`CHATTO_MAINTAINER_USER_IDS`. The maintainer check applies when a Chatto
conversation delegates work; a local invocation uses the operator's authority.

## Investigations

`investigate` gives a question to an investigator agent, which reads the
source checkout, its documentation, and its Git history, and answers with file
references. At most three investigations run at the same time in one thread.
The investigator can only read. The bot checks each `path:line` reference
against the checkout and, when GitHub is configured, links it to the lines on
GitHub at the investigated commit.

Each investigation keeps its agent session in `.runling/subagents/investigate/`.
The bot can resume an investigation with a follow-up question, also after the
investigation ended. You can also run the investigator on its own:

```sh
runling run tasks/investigate.ts --input '{"directory": "/path/to/chatto", "question": "..."}'
```

Add `"session": "<id>"` to the input to continue an investigation.

## Run it

Copy `.env.example` to `.env` and set at least `CHATTO_URL`, `CHATTO_API_KEY`,
and `CHATTO_AGENT_MODEL`. Then run `mise dev-chattobot` from the repository
root. The Runling console URL is in the output.

Create the API key for a bot account in Chatto. The bot can only read and post
where that account can.

## Settings

| Setting                                                                                         | Description                                                                                                    |
| ----------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------------------------- |
| `CHATTO_URL`, `CHATTO_API_KEY`                                                                  | The Chatto server and the bot account's API key. Required.                                                     |
| `CHATTO_AGENT_MODEL`                                                                            | The model, as `provider/model`, for example `openrouter/openai/gpt-5.5`. Required.                             |
| `CHATTO_AGENT_THINKING`                                                                         | Reasoning effort: `off`, `minimal`, `low`, `medium` (default), `high`, `xhigh`, or `max`.                      |
| `CHATTO_ALLOWED_USER_ID`                                                                        | Answer only this Chatto user ID. Use it for tests on a shared server.                                          |
| `CHATTO_MAINTAINER_USER_IDS`                                                                    | Chatto user IDs of maintainers, separated by commas.                                                           |
| `CHATTO_GITHUB_APP_CLIENT_ID`, `CHATTO_GITHUB_APP_PRIVATE_KEY_FILE`, `CHATTO_GITHUB_REPOSITORY` | GitHub access through a GitHub App, for one repository (`owner/repo`).                                         |
| `CHATTO_TAVILY_API_KEY`                                                                         | Enables `webSearch`.                                                                                           |
| `CHATTO_SOURCE_DIRECTORY`                                                                       | A checkout of the Chatto source. Enables `investigate`. Keep it current yourself, for example with `git pull`. |
| `CHATTO_INVESTIGATION_MODEL`, `CHATTO_INVESTIGATION_THINKING`                                   | The investigator's model (default: `CHATTO_AGENT_MODEL`) and reasoning effort (default: `medium`).             |
| `CHATTO_CLOUDFLARE_ACCOUNT_ID`, `CHATTO_CLOUDFLARE_API_TOKEN`                                   | Enables `browsePage`.                                                                                          |

Model providers use their own settings, for example `OPENROUTER_API_KEY`, or a
Pi login on the bot host.

### GitHub App

Create a GitHub App and install it on the repository only. Give it these
repository permissions: Issues (read and write), Pull requests, Actions,
Checks, and Commit statuses (read). ChattoBot creates a token for each command,
limited to the repository and to the permissions that the command needs. Read
commands get read-only tokens. The private key stays on the bot host.

## Privacy

ChattoBot sends these data to other systems:

- **The model provider** receives the messages to the bot, the other new
  messages in the thread, the attachments that the bot views, and all tool
  results.
- **The Chatto documentation sites and GitHub** receive the requests of
  `fetchPage` and the bot host's IP address.
- **GitHub** receives each `gh` command, the changes that the bot makes, and the
  bot host's IP address.
- **Tavily** receives search queries, and **Cloudflare** receives page URLs,
  when they are configured.
- **The model provider** of the investigator receives the questions and the
  source code that the investigator reads.
- **The model provider** of the GitHub worker receives the maintainer request,
  the context that the conversation supplies, and the GitHub tool results.

The Runling run journals in `.runling/runs/` contain the messages and replies.
The sessions in `.runling/subagents/` contain investigation questions and source
code, and GitHub worker requests, context, and tool results.
Keep that directory private.

## Develop

- `mise check-chattobot` checks the types.
- `mise test-chattobot` runs the tests.

### GitHub model evaluations

From `packages/chattobot/`, run:

```sh
pnpm eval:github
pnpm eval:github ambiguous-moo
```

These opt-in evaluations use `CHATTO_AGENT_MODEL` and
`CHATTO_AGENT_THINKING` from `.env`, with the model provider's credentials.
They contact the model provider and incur model costs. They use synthetic
messages and an in-memory GitHub fixture; they do not contact GitHub or Chatto
and do not load GitHub App credentials.

Worker cases cover ambiguous input, explicit searches, issue creation and
edits, instructions hidden in issue content, and a lost response after a
successful write. Supervisor cases cover maintainer and member requests,
including both authors in one turn and an unrelated maintainer message.
They check commands, changed issue state, and delegation targets. They do not
require exact reply wording. The supervisor cases use a fake child worker;
the worker cases use the real worker agent.

Each case runs once, with a two-minute deadline and a limit of 16 fixture
commands. A failed assertion or an unsupported fixture command fails the run.
Reports and worker sessions are saved under `.runling/evaluations/`.
The report includes the model, commands, replies, state, and failure details.
The fixture supports a small issue CLI and REST subset, not the complete
GitHub API. A passing run is a behavioral sample, not proof of authorization
or a live GitHub integration test. These evaluations are separate from the
deterministic tests in `mise test-chattobot`.

Read [AGENTS.md](AGENTS.md) before you add a tool or change the conversation.
