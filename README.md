# Sub-Nebula

Transparent proxy for Claude Code that routes:
- **Anthropic models (Claude)** → Anthropic API using your Claude Pro OAuth session
- **Subagents** → Kimi API (cheaper)

## Features

- ✅ 100% transparent - does not modify payloads
- ✅ Reads OAuth token directly from Claude Code credentials
- ✅ Token caching to avoid repeated reads
- ✅ macOS (Keychain) and Linux/Windows (file) support

## Installation

```bash
cd ~/Projects/local/sub-nebula
go mod tidy
```

## Configuration

1. Copy the example file:
```bash
cp .env.example .env
```

2. Edit `.env` with your Kimi API key:
```env
KIMI_API_KEY=sk-kimi-your-api-key-here
```

## Usage

### 1. Start the proxy

```bash
go run main.go
# or with environment variables:
KIMI_API_KEY=sk-xxx go run main.go
```

### 2. Configure Claude Code

```bash
# Create configuration to use the proxy
mkdir -p ~/.asterisk/nebula
cat > ~/.asterisk/nebula/settings.json << 'EOF'
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:4242",
    "ANTHROPIC_AUTH_TOKEN": "dummy",
    "CLAUDE_CODE_SUBAGENT_MODEL": "kimi-for-coding"
  }
}
EOF
```

### 3. Run Claude Code with the proxy

```bash
CLAUDE_CONFIG_DIR=~/.asterisk/nebula claude
```

## How it works

```
Claude Code → Sub-Nebula Proxy → Anthropic API (Claude)
                    ↓
              Kimi API (subagents)
```

The proxy detects the model in the request:
- If `claude-3-*` → Anthropic API with your OAuth token
- If `kimi-for-coding` → Kimi API with your API key

## OAuth Token Retrieval

The proxy automatically reads the token from:

- **macOS**: Keychain entry `"Claude Code-credentials"`
- **Linux/Windows**: `~/.claude/.credentials.json`

No configuration needed - uses your existing Claude Code session.

## Project Structure

```
sub-nebula/
├── main.go          # Main proxy
├── go.mod           # Dependencies
├── .env.example     # Example configuration
└── README.md        # This file
```

## Troubleshooting

### "failed to read Claude credentials"
- Make sure you have logged into Claude Code at least once
- On macOS: verify `security find-generic-password -s "Claude Code-credentials" -w` works
- On Linux: verify `~/.claude/.credentials.json` exists

### "proxy error"
- Verify the proxy is running: `curl http://localhost:4242/health`
- Check your API keys in `.env`

## License

MIT - Personal use
