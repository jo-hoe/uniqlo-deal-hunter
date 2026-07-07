# uniqlo-deal-hunter

A Kubernetes-native deal hunter for the Uniqlo DE online store. Periodically
polls Uniqlo's product API for discounted items, filters them against
user-defined rules (regex on name, size, EUR ceiling, minimum discount
percent), and sends a bullet-list email digest of new deals via SMTP.
Persists notification state in SQLite so the same deal is never announced
twice.

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
  --version 0.4.0 \
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
  segment: men
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
    # AUTH is off unless username is set. Chart users configure this
    # via notifier.smtp.auth.* (see the chart README). At the app layer
    # you either provide both username + passwordFile, or neither.
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

MIT with a no-AI-training rider — see [LICENSE](LICENSE). This code, and
any dataset that includes it, must not be used to train or improve machine
learning models.
