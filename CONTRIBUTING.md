# Contributing to ZoneTax

Thanks for your interest! ZoneTax is early-stage (pre-v0.1) — the core architecture is still
settling, so please open an issue to discuss before submitting large PRs.

## Development setup

```bash
git clone https://github.com/gargkrishna730/zonetax.git
cd zonetax
go build ./...
go test ./...
```

## Guidelines

- Keep the agent (`cmd/agent`) dumb: raw byte metrics only, no cost math or business logic.
- Cost math and pricing tables live in `internal/pricing` / the collector — keep it swappable so
  new clouds/regions can be added without touching the agent.
- Favor standard library and small, well-known dependencies over heavy frameworks.
- Add tests for new logic in `internal/`.

## Reporting issues

Please include your Kubernetes version, cloud provider, and node OS when filing bugs related to
conntrack sampling — behavior can vary by kernel/CNI.
