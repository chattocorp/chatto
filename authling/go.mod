module hmans.de/authling

go 1.26.0

tool google.golang.org/protobuf/cmd/protoc-gen-go

require (
	github.com/a-h/templ v0.3.1020
	github.com/go-jose/go-jose/v4 v4.1.4
	github.com/nats-io/nats-server/v2 v2.14.3
	github.com/nats-io/nats.go v1.52.0
	github.com/rs/cors v1.11.1
	github.com/wneessen/go-mail v0.8.1
	github.com/zitadel/oidc/v3 v3.48.1
	golang.org/x/crypto v0.54.0
	golang.org/x/sync v0.22.0
	golang.org/x/text v0.40.0
	google.golang.org/protobuf v1.36.11
	hmans.de/chatto/pkg/appconfig v0.0.0
	hmans.de/chatto/pkg/datacrypto v0.0.0
	hmans.de/chatto/pkg/events v0.0.0
	hmans.de/chatto/pkg/natsruntime v0.0.0
)

tool github.com/a-h/templ/cmd/templ

require (
	github.com/a-h/parse v0.0.0-20250122154542-74294addb73e // indirect
	github.com/andybalholm/brotli v1.1.0 // indirect
	github.com/antithesishq/antithesis-sdk-go v0.7.2 // indirect
	github.com/bmatcuk/doublestar/v4 v4.10.0 // indirect
	github.com/caarlos0/env/v11 v11.4.1 // indirect
	github.com/cenkalti/backoff/v4 v4.3.0 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/cli/browser v1.3.0 // indirect
	github.com/fatih/color v1.16.0 // indirect
	github.com/fsnotify/fsnotify v1.7.0 // indirect
	github.com/go-chi/chi/v5 v5.3.1 // indirect
	github.com/go-logr/logr v1.4.3 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/google/go-tpm v0.9.8 // indirect
	github.com/gorilla/securecookie v1.1.2 // indirect
	github.com/klauspost/compress v1.19.1 // indirect
	github.com/mattn/go-colorable v0.1.13 // indirect
	github.com/mattn/go-isatty v0.0.20 // indirect
	github.com/minio/highwayhash v1.0.4 // indirect
	github.com/muhlemmer/gu v0.3.1 // indirect
	github.com/muhlemmer/httpforwarded v0.1.0 // indirect
	github.com/natefinch/atomic v1.0.1 // indirect
	github.com/nats-io/jwt/v2 v2.8.2 // indirect
	github.com/nats-io/nkeys v0.4.16 // indirect
	github.com/nats-io/nuid v1.0.1 // indirect
	github.com/pelletier/go-toml/v2 v2.4.3 // indirect
	github.com/zitadel/schema v1.3.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel v1.44.0 // indirect
	go.opentelemetry.io/otel/metric v1.44.0 // indirect
	go.opentelemetry.io/otel/trace v1.44.0 // indirect
	golang.org/x/mod v0.37.0 // indirect
	golang.org/x/net v0.56.0 // indirect
	golang.org/x/oauth2 v0.36.0 // indirect
	golang.org/x/sys v0.47.0 // indirect
	golang.org/x/time v0.15.0 // indirect
	golang.org/x/tools v0.47.0 // indirect
)

replace hmans.de/chatto/pkg/events => ../pkg/events

replace hmans.de/chatto/pkg/appconfig => ../pkg/appconfig

replace hmans.de/chatto/pkg/datacrypto => ../pkg/datacrypto

replace hmans.de/chatto/pkg/natsruntime => ../pkg/natsruntime
