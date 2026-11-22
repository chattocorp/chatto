#!/usr/bin/env bash
# SPDX-FileCopyrightText: 2026 ChattoCorp GmbH
# SPDX-License-Identifier: AGPL-3.0-or-later

# Print the health and NATS load signals of one Chatto deployment from
# Prometheus. Use it for a canary: run it before a deploy and again after,
# then compare the two outputs.
#
# The script only reads. It sends instant queries to Prometheus through the
# Kubernetes API server proxy, so it needs no port-forward.
#
# Requirements: kubectl with read access to the Prometheus service, python3,
# the Chatto metrics endpoint scraped by Prometheus, and, for the consumer
# signals, the NATS Prometheus exporter with JetStream consumer metrics.
#
# Usage:
#   tools/canary-metrics.sh --namespace <chatto namespace> [options]
#
# Options:
#   --namespace <ns>       Kubernetes namespace of the Chatto pods. Required.
#   --account <name>       NATS account of the deployment, for the consumer
#                          signals. Without it, those signals are skipped.
#   --context <name>       kubectl context. Default: the current context.
#   --window <duration>    Rate window, for example 5m or 1h. Default: 1h.
#   --prometheus <ns/svc:port>
#                          Prometheus service. Default:
#                          monitoring/prometheus-kube-prometheus-prometheus:9090
set -euo pipefail

namespace=""
account=""
context=""
window="1h"
prometheus="monitoring/prometheus-kube-prometheus-prometheus:9090"

while [ $# -gt 0 ]; do
	case "$1" in
	--namespace) namespace="$2"; shift 2 ;;
	--account) account="$2"; shift 2 ;;
	--context) context="$2"; shift 2 ;;
	--window) window="$2"; shift 2 ;;
	--prometheus) prometheus="$2"; shift 2 ;;
	-h | --help) sed -n '5,28p' "$0" | sed 's/^# \{0,1\}//'; exit 0 ;;
	*) echo "unknown argument: $1" >&2; exit 2 ;;
	esac
done
if [ -z "$namespace" ]; then
	echo "--namespace is required" >&2
	exit 2
fi
# The values go into PromQL label matchers and a URL path. Accept only the
# characters that Kubernetes names, NATS account names, and durations use.
for value in "$namespace" "$account" "$window"; do
	if [[ ! "$value" =~ ^[A-Za-z0-9._-]*$ ]]; then
		echo "invalid value: $value" >&2
		exit 2
	fi
done
if [[ ! "$prometheus" =~ ^[a-z0-9-]+/[a-z0-9-]+:[0-9]+$ ]]; then
	echo "--prometheus must have the form <namespace>/<service>:<port>" >&2
	exit 2
fi

kubectl_args=()
if [ -n "$context" ]; then
	kubectl_args+=(--context "$context")
fi
api="/api/v1/namespaces/${prometheus%%/*}/services/${prometheus#*/}/proxy/api/v1/query"
ns="namespace=\"$namespace\""
workers="account=\"$account\",consumer_name=~\"chatto-.*\""

# query <label> <PromQL> prints one line: the label, then each result as
# <distinguishing label values>=<value>.
query() {
	local encoded
	encoded="$(python3 -c 'import sys, urllib.parse; print(urllib.parse.quote(sys.argv[1]))' "$2")"
	printf '%-44s ' "$1"
	kubectl "${kubectl_args[@]}" get --raw "$api?query=$encoded" | python3 -c '
import json, sys

shown = ("version", "direction", "kind", "reason", "pod", "consumer_name")
results = json.load(sys.stdin)["data"]["result"]
parts = []
for result in results:
    name = "/".join(v for k, v in sorted(result["metric"].items()) if k in shown) or "value"
    parts.append("%s=%.3f" % (name, float(result["value"][1])))
print(", ".join(sorted(parts)) or "(no series)")
'
}

echo "# Chatto"
query "build" "count by (version)(chatto_build_info{$ns})"
query "pod uptime, seconds" "time() - process_start_time_seconds{$ns}"
query "ready (minimum of pods)" "min(chatto_ready{$ns})"
query "NATS connected (minimum of pods)" "min(chatto_nats_connected{$ns})"
query "NATS reconnects in window" "sum(increase(chatto_nats_reconnects_total{$ns}[$window]))"
query "projection stalled (maximum)" "max(chatto_projection_stalled{$ns})"
query "projection lag, events (maximum)" "max(chatto_projection_lag_events{$ns})"
query "realtime connections" "sum(chatto_realtime_websocket_connections{$ns})"

echo "# NATS traffic of the Chatto pods"
query "messages per second" "sum by (direction)(rate(chatto_nats_messages_total{$ns}[$window]))"
query "bytes per second" "sum by (direction)(rate(chatto_nats_bytes_total{$ns}[$window]))"

echo "# Auth generation checks (ADR-127)"
query "checks in window" "sum by (kind)(increase(chatto_auth_generation_checks_total{$ns}[$window]))"
query "strict checks in window, by reason" "sum by (reason)(increase(chatto_auth_generation_strict_checks_total{$ns}[$window]))"

echo "# Process"
query "CPU, cores" "rate(process_cpu_seconds_total{$ns}[$window])"
query "allocation, MB per second" "rate(go_memstats_alloc_bytes_total{$ns}[$window]) / 1e6"
query "heap in use, MB" "go_memstats_heap_inuse_bytes{$ns} / 1e6"
query "goroutines" "go_goroutines{$ns}"

if [ -n "$account" ]; then
	echo "# Durable workers (NATS account $account)"
	query "durable consumers" "count(max by (stream_name, consumer_name)(nats_consumer_num_pending{$workers}))"
	query "pending work, maximum now" "max(nats_consumer_num_pending{$workers})"
	query "pending work, peak in window" "max(max_over_time(nats_consumer_num_pending{$workers}[$window]))"
	query "unacknowledged work, peak in window" "max(max_over_time(nats_consumer_num_ack_pending{$workers}[$window]))"
	query "redeliveries in window" "sum(max by (stream_name, consumer_name)(increase(nats_consumer_num_redelivered{$workers}[$window])))"
	query "consumers with no waiting pull" "max by (consumer_name)(nats_consumer_num_waiting{$workers}) < 1"
else
	echo "# Durable workers: skipped (no --account)"
fi
