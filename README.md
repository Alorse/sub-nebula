# Sub-Nebula

**Reduce Claude Code token costs by routing subagents to cheaper models.**

Paid Claude Code accounts burn tokens fast. This proxy cuts costs by keeping your main Claude interactions on Anthropic while routing subagent tasks to Kimi (significantly cheaper).

## Quick Start

```bash
# 1. Configure - copy .env.example to .env and edit
cp .env.example .env
# Edit .env: KIMI_API_KEY=sk-kimi-xxx

# 2. Start proxy
go run main.go

# 3. Configure Claude Code
cat > ~/.claude/settings.json << 'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:4242",
    "ANTHROPIC_AUTH_TOKEN": "dummy",
    "CLAUDE_CODE_SUBAGENT_MODEL": "kimi-for-coding"
  }
}
EOF

# 4. Run with proxy
claude
```

## How It Works

```
┌─────────────┐     ┌──────────────┐     ┌─────────────────┐
│ Claude Code │────▶│ Sub-Nebula   │────▶│ Anthropic API   │
│             │     │ Proxy :4242  │     │ (Claude models) │
└─────────────┘     └──────────────┘     └─────────────────┘
                           │
                           ▼
                    ┌──────────────┐
                    │ Kimi API     │
                    │ (subagents)  │
                    └──────────────┘
```

Routing logic:
- `claude-*` models → Anthropic API (your OAuth session)
- `kimi-*` models → Kimi API (your API key)

## Features

- **Transparent proxy** - No payload modification
- **OAuth auto-auth** - Reads Claude token from system credentials (macOS Keychain / `~/.claude/.credentials.json`)
- **Token caching** - Avoids repeated credential reads
- **`/v1/models` endpoint** - Lists available models from both providers
- **Concurrent fetching** - Aggregates models from Anthropic + Kimi in parallel

## Endpoints

| Endpoint | Description |
|----------|-------------|
| `GET /health` | Proxy health check |
| `GET /v1/models` | List all available models |
| `POST /v1/messages` | Proxy messages to appropriate provider |

## Environment Variables

| Variable | Default | Description |
|----------|---------|-------------|
| `PORT` | `4242` | Proxy port |
| `KIMI_API_KEY` | - | Kimi API key |
| `KIMI_BASE_URL` | `https://api.kimi.com/coding` | Kimi endpoint |
| `SUBAGENT_MODEL` | `kimi-for-coding` | Default subagent model |

## Troubleshooting

**"failed to read Claude credentials"**
- Ensure you've logged into Claude Code at least once
- macOS: verify `security find-generic-password -s "Claude Code-credentials" -w` works
- Linux: verify `~/.claude/.credentials.json` exists

**"401 Unauthorized"**
- Check your OAuth session is valid: `claude login`
- Verify `KIMI_API_KEY` is set correctly

## License

MIT
