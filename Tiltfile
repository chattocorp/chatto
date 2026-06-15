frontend_port = int(os.getenv('VITE_PORT', '5173'))
backend_port = int(os.getenv('CHATTO_WEBSERVER_PORT', '4000'))
nats_port = int(os.getenv('CHATTO_NATS_EMBEDDED_PORT', '4555'))
metrics_port = int(os.getenv('CHATTO_METRICS_PORT', '9090'))
webserver_url = os.getenv('CHATTO_WEBSERVER_URL', 'http://localhost:%d' % frontend_port)
metrics_url = 'http://localhost:%d/metrics' % metrics_port

local_resource(
    'frontend-deps',
    cmd='mise run deps-frontend',
    deps=['frontend/package.json', 'frontend/pnpm-lock.yaml'],
    allow_parallel=True,
)

local_resource(
    'backend',
    cmd='',
    serve_cmd='mise run dev-backend',
    serve_dir='cli',
    serve_env={
        'CHATTO_WEBSERVER_PORT': str(backend_port),
        'CHATTO_WEBSERVER_URL': webserver_url,
        'CHATTO_NATS_EMBEDDED_PORT': str(nats_port),
        'CHATTO_METRICS_ENABLED': 'true',
        'CHATTO_METRICS_BIND_ADDRESS': '127.0.0.1',
        'CHATTO_METRICS_PORT': str(metrics_port),
        'CHATTO_METRICS_PATH': '/metrics',
    },
    deps=['cli'],
    ignore=[
        'cli/bin',
        'cli/data',
        'cli/internal/http_server/.client',
    ],
    readiness_probe=probe(
        period_secs=1,
        timeout_secs=1,
        tcp_socket=tcp_socket_action(port=backend_port),
    ),
    links=[webserver_url, metrics_url],
    allow_parallel=True,
)

local_resource(
    'frontend',
    cmd='',
    serve_cmd='mise run dev-frontend',
    serve_dir='frontend',
    serve_env={
        'VITE_PORT': str(frontend_port),
        'CHATTO_WEBSERVER_PORT': str(backend_port),
    },
    resource_deps=['frontend-deps'],
    readiness_probe=probe(
        period_secs=1,
        timeout_secs=1,
        tcp_socket=tcp_socket_action(port=frontend_port),
    ),
    links=[webserver_url],
    allow_parallel=True,
)

local_resource(
    'frontend-codegen',
    cmd='',
    serve_cmd='mise run dev-frontend-codegen',
    serve_dir='frontend',
    resource_deps=['frontend-deps'],
    allow_parallel=True,
)
