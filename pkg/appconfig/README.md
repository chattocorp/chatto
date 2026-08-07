# Application Configuration

`hmans.de/chatto/pkg/appconfig` loads application-owned Go configuration
structs from TOML files and then applies struct-tagged environment-variable
overrides.

The caller selects its explicit and default file paths and whether an explicit
file is required or unknown TOML fields are rejected. Applications retain
ownership of configuration schemas, environment names, defaults,
normalization, validation, compatibility aliases, and generated examples.

## Status

This module is an incubation surface shared by Chatto and Authling. Its API is
not yet covered by a stability promise, and releases remain pre-1.0 while both
applications establish the smallest useful contract.

## Development

Run the module tests independently:

```sh
mise test-appconfig
```

From this directory, the equivalent standalone check is:

```sh
GOWORK=off go test ./...
```

## License

The module is licensed under [`Apache-2.0`](LICENSE). Its permissive license
does not imply API stability.
