# MCP configuration lifecycle

`mcpclient.NewManager` prepares a manager without connecting servers.
`Manager.Configure(Config)` validates and copies a replacement declarative
catalog. Failure preserves the previous catalog; a closed manager rejects it.
Configuration performs no network operations and leaves active connections
unchanged. `Start` connects one snapshot of configured servers, while
`ConnectConfigured` uses the current catalog for manual reconnect.
`Configured()` returns a detached copy so callers can merge additions while
preserving prepared server settings and the injected credential resolver.

This allows the public Agent to merge trusted namespaced plugin MCP declarations
into an injected manager before `Components.Start`, keeping backend construction
and application activation separate. Plugin parsing, trust and collision policy
remain in the caller; this package has no dependency on the plugin module or Agent.
Existing credential resolution, publication, approval and connection cleanup
contracts remain in place. There are no new configuration fields.

Each server uses a 30-second client deadline when `timeout_seconds` is unset;
set `mcp.servers.<name>.timeout_seconds` for browser automation or other tools
that can legitimately run longer. A client deadline is reported separately
from caller cancellation and protocol/tool errors, and the underlying error is
wrapped for logs and callers to inspect. MCP adapters require the
context-aware tool path so the agent request deadline reaches the server;
legacy context-free registry dispatch is intentionally unavailable for MCP
tools.

```bash
GOWORK=off go test -race ./mcpclient ./bootstrap
GOWORK=off go vet ./mcpclient ./bootstrap
```
