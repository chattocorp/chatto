---
type: feature
---

Usernames can contain up to 64 characters by default. Set `core.accounts.max_login_length` in TOML or `CHATTO_CORE_ACCOUNTS_MAX_LOGIN_LENGTH` in the environment to choose a limit from 2 to 254 characters. Archive import checks new usernames against the destination limit before it writes any records.

Set the verified account quota with `core.accounts.max_users` or `CHATTO_CORE_ACCOUNTS_MAX_USERS`. Existing quota settings continue to work.
