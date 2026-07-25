# Archery MCP Server

[中文文档](README.zh.md)

MCP server for the fixed Archery 1.14.0 public API surface. It uses one configured Archery service account and exposes 31 named tools. TOTP setup and save operations are intentionally excluded.

## Quick start

```bash
# Requires Go 1.25+ (see .tool-versions)
make build
./bin/archery-mcp-server version

# Start MCP server (stdio, default)
./bin/archery-mcp-server mcp

# List / call tools from the CLI (no MCP client required)
./bin/archery-mcp-server tools list
MCP_ARCHERY_PASSWORD='provided-at-runtime' ./bin/archery-mcp-server --config config.example.yaml tools list
```

## Features

- **Subcommand CLI**: `mcp` starts the server; `tools` / `version` / `completion` are separate commands
- **Multi-transport**: stdio, Streamable HTTP, and SSE
- **Tool filtering**: `--enabled-tools`, `--disabled-tools`, `--enable-domains`, `--disable-domains`
- **Engineering defaults**: Makefile, Dockerfile, GitHub Actions CI, golangci-lint config, MIT license

## CLI commands

| Command | Purpose |
|---------|---------|
| `archery-mcp-server mcp` | Start the MCP server (stdio or HTTP) |
| `archery-mcp-server tools list` | List enabled tools |
| `archery-mcp-server tools describe <name>` | Show tool schema |
| `archery-mcp-server tools call <name>` | Invoke a tool with JSON params |
| `archery-mcp-server version` | Print build metadata |
| `archery-mcp-server completion <shell>` | Generate shell completion |

### Tool examples

```bash
./bin/archery-mcp-server tools list
./bin/archery-mcp-server tools list --json
./bin/archery-mcp-server --config config.example.yaml tools describe workflows.sql_check
```

## MCP transports

### Stdio (default)

```bash
./bin/archery-mcp-server mcp
```

Cursor / Claude Desktop style config:

```json
{
  "mcpServers": {
    "archery-mcp-server": {
      "command": "/absolute/path/to/bin/archery-mcp-server",
      "args": ["mcp"]
    }
  }
}
```

### Streamable HTTP

```bash
./bin/archery-mcp-server mcp --port 8080
curl -s http://127.0.0.1:8080/healthz
```

Client config example:

```json
{
  "mcpServers": {
    "archery-mcp-server": {
      "url": "http://127.0.0.1:8080/mcp"
    }
  }
}
```

### SSE

Same process as HTTP mode. Endpoints:

| Path | Purpose |
|------|---------|
| `/healthz` | Health check (`GET`/`HEAD`) |
| `/mcp` | Streamable HTTP |
| `/sse` | SSE connection |
| `/message` | SSE message endpoint |

```bash
./bin/archery-mcp-server mcp --port 8080 --sse-base-url http://127.0.0.1:8080
```

### Docker

```bash
# Build
make docker

# Stdio (default ENTRYPOINT is `archery-mcp-server mcp`)
docker run -i --rm archery-mcp-server:dev

# HTTP
docker run --rm -p 8080:8080 archery-mcp-server:dev --port 8080 --listen 0.0.0.0
```

HTTP / SSE have **no auth and no TLS**. Default `--listen 127.0.0.1`. Use only on trusted networks; put a reverse proxy in front if you expose the port.

## Configuration

Priority: **flags > environment variables > config file > defaults**.

### Environment variables

| Variable | Description | Default |
|----------|-------------|---------|
| `MCP_LOG_LEVEL` | Log level | `info` |
| `MCP_PORT` | HTTP port (`0` = stdio) | `0` |
| `MCP_LISTEN` | HTTP listen host | `127.0.0.1` |
| `MCP_SSE_BASE_URL` | Public SSE base URL | `""` |

### Config file

See `config.example.yaml`:

```yaml
port: 0
listen: 127.0.0.1
sse_base_url: ""
log_level: info
enabled_tools: []
disabled_tools: []
enabled_domains: []
disabled_domains: []
archery:
  base_url: https://archery.example.com/prefix
  username: mcp-service
  password_env: MCP_ARCHERY_PASSWORD
  request_timeout: 15s
```

```bash
./bin/archery-mcp-server mcp --config config.example.yaml --port 8080
```

## Project layout

```
cmd/mcp-server/          # binary entrypoint
internal/cmd/             # cobra CLI (mcp / tools / version / completion)
pkg/core/                 # config / logging / version
pkg/server/mcp/           # MCP registration and transports
pkg/server/http/          # HTTP / SSE / healthz
pkg/toolset/              # Toolset interface and filters
pkg/toolset/archery/      # fixed Archery tool surface
.github/workflows/        # CI
```

## Extend with your own tools

1. Add `pkg/toolset/<your-domain>/` implementing `toolset.Toolset`.
2. Register it in `internal/cmd/root.go` via `defaultToolsets()`.
3. Control exposure with `--enabled-tools` / `--enable-domains`.

## Development

```bash
make tidy
make format
make lint
make test
make coverage
make ci
make build
make docker
```

CI (`.github/workflows/build.yaml`) runs lint, race tests with coverage upload, multi-OS build, CLI smoke (`version` / `tools list`), and Docker image build.

## Local reference clones

`third-party-projects/` can hold local reference checkouts. It is listed in `.gitignore` and is not part of the template deliverable.

## License

[MIT](LICENSE)
