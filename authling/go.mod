module hmans.de/authling

go 1.26.0

tool google.golang.org/protobuf/cmd/protoc-gen-go

require (
	github.com/nats-io/nats-server/v2 v2.14.3
	github.com/nats-io/nats.go v1.52.0
	google.golang.org/protobuf v1.36.11
	hmans.de/chatto/pkg/appconfig v0.0.0
	hmans.de/chatto/pkg/datacrypto v0.0.0
	hmans.de/chatto/pkg/events v0.0.0
	hmans.de/chatto/pkg/natsruntime v0.0.0
)

tool github.com/a-h/templ/cmd/templ

require (
	github.com/a-h/parse v0.0.0-20250122154542-74294addb73e // indirect
	github.com/a-h/templ v0.3.1020 // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/antithesishq/antithesis-sdk-go v0.7.2 // indirect
	github.com/caarlos0/env/v11 v11.4.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cli/browser v1.3.0 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/natefinch/atomic v1.0.1 // indirect
	github.com/nats-io/jwt/v2 v2.8.2 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	golang.org/x/crypto v0.54.0 // indirect
	golang.org/x/mod v0.33.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/sync v0.19.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.42.0 // indirect
)

replace hmans.de/chatto/pkg/events => ../pkg/events

replace hmans.de/chatto/pkg/appconfig => ../pkg/appconfig

replace hmans.de/chatto/pkg/datacrypto => ../pkg/datacrypto

replace hmans.de/chatto/pkg/natsruntime => ../pkg/natsruntime
