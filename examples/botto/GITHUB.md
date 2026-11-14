# GitHub access for Klaus

Botto uses the official GitHub MCP server through its existing MCP connection.
The server runs locally as a child process. No remote MCP endpoint is required.

## Prepare the App

Use a GitHub App installation selected for `chattocorp/chatto`. Give it Issues
read/write, Pull requests read, and Contents read permissions. Do not grant code
writes or repository administration for this setup. An installation with access
to other repositories can also authorize operations there; the skill's default
repository is not an access control. Public repository reads can remain available.

Install an official `github-mcp-server` release that supports GitHub App
authentication. Record the version you install. Check its `stdio --help` output
for `--app-id`, `--app-installation-id`, and `--app-private-key-path` before use.
See the [release downloads](https://github.com/github/github-mcp-server/releases)
and [App authentication instructions](https://github.com/github/github-mcp-server/blob/main/docs/github-app-auth.md).

## Add local credentials

Export the App's private key from 1Password to `examples/botto/github-app.pem`.
Use the private key, not a public certificate. From the repository root, restrict
its file permissions:

```sh
chmod 600 examples/botto/github-app.pem
```

Copy the entries from `.env.github.example` into the existing `.env`, and replace
the placeholders with the executable path, App ID, installation ID, and absolute
key path. Keep the key contents out of `.env`. Both `.env` and `*.pem` are ignored
by Git in this directory. Do not copy credentials into a tracked example file.
The MCP process inherits the host environment. Remove alternative PAT or OAuth
authentication settings from that environment so it uses the App configuration.

Enable the commented GitHub block in `botto.toml`. If it is missing, copy the
GitHub block from `botto.example.toml` and uncomment it. Keep only one GitHub MCP
table. In the existing `[chatto]` table, set
`admin_user_ids` to the stable Chatto IDs of the owners who may request writes.
This is an explicit operator list, not automatic Chatto or GitHub role discovery.
Restart Botto after the credentials and configuration are complete.

## Access and verification

The template exposes issue, PR, and source reads to ordinary conversations.
Only batches authored entirely by configured admins receive `issue_write` and
`add_issue_comment`. Mixed batches receive read tools only. Botto checks tool
access before execution, including in restored conversations. Issue write access
includes creation and updates such as closing an issue. No PR write, merge, or
code write tool is exposed.

The GitHub skill requires an explicit request for a change. The model resolves
the requested scope from the conversation; the identity check alone does not
prove that every argument matches the owner's intent.

After startup:

1. Ask Klaus to read a known `chattocorp/chatto` issue and PR, and return links.
2. From a non-owner account, request an issue change. Klaus must not execute it.
3. From an owner account, explicitly request a disposable test issue. Check the
   created issue, then request an edit and closure. These checks write to GitHub.
4. Restart Botto and check another read. Confirm that an old owner message does
   not grant write access to a new non-owner request.

Use a repository read to check App access, not `get_me`. Do not infer successful
delivery from a model reply alone; check the tool result and GitHub issue. If a
write has an uncertain outcome, inspect the issue before repeating the operation.

GitHub receives tool requests and the bot host's IP address. Tool results enter
the model context and can remain in local transcripts. Issue writes publish the
submitted content to the repository's audience. Share conversation details or
attachments only when that disclosure is authorized.
