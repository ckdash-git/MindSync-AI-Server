# MindSync AI Server

Production-grade Go backend for the MindSync AI multi-platform application.

## Features

- 🔒 **Privacy-First** — PII detection, tokenization, and rehydration proxy
- 🌊 **SSE Streaming** — Real-time AI response streaming
- 🔑 **Secure** — AES-256-GCM encryption with key rotation, JWT auth
- 🏗️ **Clean Architecture** — Handler → Service → Domain → Repository layers
- 🤖 **Multi-Model** — OpenRouter integration (GPT, Claude, Gemini)
- 📈 **Scalable** — Connection pooling, rate limiting, backpressure handling

## Quick Start

```bash
# Clone
git clone https://github.com/ckdash-git/MindSync-AI-Server.git
cd MindSync-AI-Server

# Setup environment
cp .env.example .env
# Edit .env with your values

# Start PostgreSQL
make db-up

# Run migrations
make migrate-up

# Run server
make dev
```

## Project Structure

```
cmd/server/          → Entry point
internal/
  config/            → Configuration loader
  logger/            → Structured JSON logging
  domain/            → Entities and interfaces
  handler/           → HTTP handlers
  service/           → Business logic
  repository/        → Database access
  middleware/         → Auth, rate limiting, logging
  privacy/           → PII detection, tokenization, rehydration
  security/          → Encryption, hashing, JWT
  openrouter/        → OpenRouter API client
pkg/response/        → Standardized API responses
scripts/             → Helper scripts
```

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check |
| POST | `/api/v1/auth/register` | Register |
| POST | `/api/v1/auth/login` | Login |
| POST | `/api/v1/auth/refresh` | Refresh token |
| POST | `/api/v1/auth/logout` | Logout |
| POST | `/api/v1/chats` | Create chat |
| GET | `/api/v1/chats` | List chats |
| GET | `/api/v1/chats/{id}` | Get chat |
| DELETE | `/api/v1/chats/{id}` | Delete chat |
| POST | `/api/v1/chats/{id}/messages` | Send message |
| POST | `/api/v1/chat/stream` | SSE streaming |

## Development

```bash
make build        # Build binary
make test         # Run tests
make lint         # Lint code
make test-cover   # Coverage report
```

## License

Proprietary — MindSync AI
