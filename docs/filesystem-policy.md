# Filesystem read and write roots

Runtime configuration keeps additional read-only directories separate from
directories where tools may write:

```json
{
  "tools": {
    "filesystem": {
      "allowed_read_roots": ["/home/me/reference"],
      "allowed_write_roots": ["/home/me/projects"]
    }
  }
}
```

`tools.filesystem.allowed_write_roots` defaults to `[]`. A missing or empty
list preserves the existing builtin write roots, including `~/.luckyagent/`
and the system temporary directories. Read roots never become write roots.
To restore that default, remove the write-root entries or set the list to
`[]`; `config.Manager.Set` uses a comma-separated list and an empty string
clears it.

A write root also allows reading its contents. Explicit roots do not bypass
the tools module's restricted credential paths. Path normalization and
enforcement belong to `luckyagent-tools`;
runtime copies the two lists into its `FilesystemPolicy` when creating tool
services. The same startup policy reaches registered filesystem tools and
the terminal's supported static path checks.

Changes require rebuilding the runtime services, normally by restarting the
application. Saving or reloading configuration does not replace policies
captured by already registered tools; reload classification reports
`runtime_services` as requiring restart.

This configuration requires a tools dependency that implements
`FilesystemPolicy.AllowedWriteRoots`. The runtime module pins a published,
downloadable provider commit in `go.mod`; local replacements are not needed.
The launcher owns its separate `config.example.json` and product documentation.
Updating that third repository is a separate consumer change; this runtime
change defines the exact JSON contract above without changing the launcher.
