# uniqlo-deal-hunter

A Kubernetes-native deal hunter for the Uniqlo DE online store. Periodically
polls Uniqlo's product API for discounted items, filters them against
user-defined rules (regex on name, size, EUR ceiling, minimum discount
percent), and sends a bullet-list email digest of new deals via SMTP.
Persists notification state in SQLite so the same deal is never announced
twice.

## Highlights

- **No headless browser** — talks to Uniqlo's public JSON API directly.
  Small container (~15 MB, distroless), fast, easy to reason about.
- **Strongly typed end-to-end**: config, API DTOs, domain, storage all use
  concrete Go types. `shopspring/decimal` for money — never `float64`.
- **Structured JSON logs** via stdlib `log/slog` — Grafana / Loki ready.
- **CronJob-first**: one binary, one pass, exits 0. No daemon.
- **Helm chart** released as an OCI artifact to GHCR.
- **Auto-generated chart docs** via `helm-docs`.
- **Local dev** with `k3d` + `Makefile`.

## Quick start (local)

```bash
make init          # go mod download
make test          # unit tests
make run           # runs one pass against dev/config.yaml
```

## Kubernetes install

```bash
helm install uniqlo-deal-hunter \
  oci://ghcr.io/jo-hoe/charts/uniqlo-deal-hunter \
  --version 0.1.0 \
  -f my-values.yaml
```

See [charts/uniqlo-deal-hunter/README.md](charts/uniqlo-deal-hunter/README.md)
for all values.

## Configuration

The app reads a single YAML file (default `/run/config/config.yaml`). Example:

```yaml
source:
  kind: uniqlo
  baseURL: https://www.uniqlo.com
  region: de
  language: en
  gender: men
  sizeCodes: ["MSC027", "SMA004"]
  sort: 2
  clientID: uq.de.web-spa
  requestsPerSecond: 1

rules:
  - name: cheap-socks
    namePattern: "(?i)socks"
    maxPriceEUR: 5.0
  - name: cashmere
    namePattern: "(?i)cashmere|merino"
    sizes: ["M", "L"]
    minDiscountPercent: 40

notifier:
  kind: smtp
  smtp:
    host: smtp.example.com
    port: 587
    startTLS: true
    from: deals@example.com
    to: ["me@example.com"]
    username: deals@example.com
    passwordFile: /run/secrets/smtp/password

store:
  kind: sqlite
  path: /var/lib/uniqlo-deal-hunter/state.db
  retentionDays: 90

logging:
  level: info
```

## License

MIT — see [LICENSE](LICENSE).
