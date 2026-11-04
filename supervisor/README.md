# Fleet supervisor

The supervisor rules live in `internal/mcp/instructions.md` and reach Copilot as the
fleet MCP server instructions. `fleet up` starts Copilot with `--no-custom-instructions`,
so this file is not loaded; it only points to the real one.
