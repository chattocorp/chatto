# ChattoBot

ChattoBot is the Chatto project's assistant. It runs in a Chatto server as a bot
account. It answers questions about Chatto from the documentation, and
maintainers can ask it to file GitHub issues.

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

| Tool             | Who can use it | What it does                                                          |
| ---------------- | -------------- | --------------------------------------------------------------------- |
| `fetchPage`      | Everyone       | Reads the Chatto documentation and the Awesome Chatto list.           |
| `readThread`     | Everyone       | Reads the newest messages of the thread.                              |
| `viewAttachment` | Everyone       | Looks at an image or a small text file in the thread.                 |
| `webSearch`      | Maintainers    | Searches the web with Tavily, when configured.                        |
| `browsePage`     | Maintainers    | Reads a web page with Cloudflare Browser Run, when configured.        |
| `gh`             | Everyone       | Runs read-only GitHub CLI commands, when GitHub is configured.        |
| `createIssue`    | Maintainers    | Files an issue in the configured repository.                          |
| `commentOnIssue` | Maintainers    | Adds a comment to an issue in the configured repository.              |
| `investigate`    | Everyone       | Asks an investigator subagent about the source code, when configured. |

A tool for maintainers runs only when maintainers wrote every message of the
current turn.

## Investigations

`investigate` works like a subagent in an agent harness. The bot gives a
question to an investigator agent, which reads the source checkout, its
documentation, and its Git history, and answers with file references. The
investigator can only read. The bot checks each `path:line` reference against
the checkout and, when GitHub is configured, links it to the lines on GitHub at
the investigated commit.

Each investigation keeps its agent session in `.runling/subagents/investigate/`.
The bot can resume an investigation with a follow-up question, also after the
investigation ended. You can also run the investigator on its own:

```sh
runling run investigation/investigate.ts --input '{"directory": "/path/to/chatto", "question": "..."}'
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
- **GitHub** receives each `gh` command, the issues that the bot files, and the
  bot host's IP address.
- **Tavily** receives search queries, and **Cloudflare** receives page URLs,
  when they are configured.
- **The model provider** of the investigator receives the questions and the
  source code that the investigator reads.

The Runling run journals in `.runling/runs/` contain the messages and replies.
The investigation sessions in `.runling/subagents/` contain the questions and
the code that the investigators read.
Keep that directory private.

## Develop

- `mise check-chattobot` checks the types.
- `mise test-chattobot` runs the tests.

Read [AGENTS.md](AGENTS.md) before you add a tool or change the conversation.
