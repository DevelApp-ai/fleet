# Fleet admiral

The admiral rules live in `internal/mcp/instructions.md` and reach Copilot as the
fleet MCP server instructions. `fleet up` starts Copilot with `--no-custom-instructions`,
so this file is not loaded; it only points to the real one.

This directory is the admiral pane's working directory. It keeps its old name
(the admiral was called the supervisor before ADR-26) because a running admiral's
cwd is this path and Copilot's folder trust is keyed by path: moving it would break
the live session and ask for trust again on the next reset. `FLEET_ADMIRAL_DIR`
overrides it.
