# uniqlo-deal-hunter

![Version: 0.3.0](https://img.shields.io/badge/Version-0.3.0-informational?style=flat-square) ![Type: application](https://img.shields.io/badge/Type-application-informational?style=flat-square) ![AppVersion: 0.2.0](https://img.shields.io/badge/AppVersion-0.2.0-informational?style=flat-square)

A Kubernetes-native deal hunter for the Uniqlo online store.

**Homepage:** <https://github.com/jo-hoe/uniqlo-deal-hunter>

## Maintainers

| Name | Email | Url |
| ---- | ------ | --- |
| jo-hoe |  | <https://github.com/jo-hoe> |

## Source Code

* <https://github.com/jo-hoe/uniqlo-deal-hunter>

## Requirements

Kubernetes: `>=1.27.0`

## Values

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| activeDeadlineSeconds | int | `900` | The Job's activeDeadlineSeconds; the pod is killed after this many seconds. |
| affinity | object | `{}` | Affinity rules for the pod. |
| backoffLimit | int | `2` | Number of retries the Job performs before giving up. |
| concurrencyPolicy | string | `"Forbid"` | CronJob concurrency policy: Allow, Forbid, or Replace. |
| extraEnv | list | `[]` | Extra environment variables for the container. |
| extraVolumeMounts | list | `[]` | Extra volume mounts. |
| extraVolumes | list | `[]` | Extra volumes. |
| failedJobsHistoryLimit | int | `3` | Number of failed finished jobs to keep. |
| fullnameOverride | string | `""` | Fully override the release-derived fullname. |
| image | object | `{"pullPolicy":"IfNotPresent","repository":"ghcr.io/jo-hoe/uniqlo-deal-hunter","tag":""}` | Container image settings. |
| image.pullPolicy | string | `"IfNotPresent"` | Image pull policy. |
| image.repository | string | `"ghcr.io/jo-hoe/uniqlo-deal-hunter"` | The image repository. |
| image.tag | string | `""` | Image tag; defaults to Chart.AppVersion if empty. |
| imagePullSecrets | list | `[]` | Names of image pull secrets to attach to the pod. Objects must exist in the same namespace. |
| logging | object | `{"level":"info"}` | Logging configuration. |
| logging.level | string | `"info"` | Level: debug, info, warn, error. |
| nameOverride | string | `""` | Override the chart name in generated resource names. |
| nodeSelector | object | `{}` | Node selector for the pod. |
| notifier | object | `{"kind":"smtp","smtp":{"auth":{"enabled":true,"existingSecret":{"key":"password","name":""},"password":"","username":"deals@example.com"},"from":"deals@example.com","host":"smtp.example.com","port":587,"startTLS":true,"timeout":"15s","to":["me@example.com"]}}` | Notifier configuration. Only SMTP is supported today. |
| notifier.smtp.auth | object | `{"enabled":true,"existingSecret":{"key":"password","name":""},"password":"","username":"deals@example.com"}` | Authentication configuration. Three mutually-exclusive modes are supported:    1. `enabled: false`      No AUTH handshake, no password mount, no k8s Secret required.      For internal cluster mail relays with authentication disabled.    2. `enabled: true` + `existingSecret.name: <name>`      Reference an out-of-band Secret (e.g. one managed by      sealed-secrets, external-secrets, or helm-secrets). The chart      mounts it at /run/secrets/smtp/<key>. NEVER commit plaintext.    3. `enabled: true` + `password: <value>`      The chart itself creates a Secret containing the given value.      Use with `helm secrets install` to keep the value encrypted at      rest, or reserve for local/dev bootstrap only. |
| notifier.smtp.auth.enabled | bool | `true` | Whether the SMTP server requires AUTH. Set to false for password-less internal relays. |
| notifier.smtp.auth.existingSecret | object | `{"key":"password","name":""}` | Reference to an existing Secret containing the password. If `name` is set, the chart mounts that Secret and will NOT create a new one; the inline `password` field is ignored. |
| notifier.smtp.auth.existingSecret.key | string | `"password"` | Key inside the Secret containing the password. |
| notifier.smtp.auth.existingSecret.name | string | `""` | Name of a pre-existing Secret in the release namespace. |
| notifier.smtp.auth.password | string | `""` | Inline password. When set (and `existingSecret.name` is empty), the chart creates a Secret containing this value. Prefer supplying via `helm secrets install -f secrets.yaml` so the file is never committed in plaintext. |
| notifier.smtp.auth.username | string | `"deals@example.com"` | SMTP AUTH username. Ignored when `auth.enabled` is false. |
| notifier.smtp.from | string | `"deals@example.com"` | From address on the outgoing message. |
| notifier.smtp.host | string | `"smtp.example.com"` | SMTP host. |
| notifier.smtp.port | int | `587` | SMTP port (typically 587 for STARTTLS, 25 for internal relays). |
| notifier.smtp.startTLS | bool | `true` | Whether to negotiate STARTTLS after EHLO. |
| notifier.smtp.timeout | string | `"15s"` | Timeout applied to the entire SMTP dialog. |
| notifier.smtp.to | list | `["me@example.com"]` | One or more recipient addresses. |
| persistence | object | `{"accessModes":["ReadWriteOnce"],"enabled":true,"existingClaim":"","retentionDays":90,"size":"100Mi","storageClass":""}` | Persistence for the SQLite state DB.  When `enabled: true`, the chart creates a PVC and mounts it into the pod at /var/lib/uniqlo-deal-hunter. State survives pod restarts, so a deal is announced at most once.  When `enabled: false`, the app runs SQLite in-memory (`:memory:`). NO PVC is created and NO volume is mounted. Every pod restart wipes the dedup state, so any deal that is still on sale will be announced again on the next CronJob run. Choose this mode only when re-notifications are acceptable or when the CronJob runs infrequently enough that duplicates are tolerable. |
| persistence.accessModes | list | `["ReadWriteOnce"]` | Access modes. Ignored when `enabled: false`. |
| persistence.enabled | bool | `true` | Enable PVC-backed persistence. When false, state is lost every run. |
| persistence.existingClaim | string | `""` | Existing PVC name to reuse (skips creating a new one). Ignored when `enabled: false`. |
| persistence.retentionDays | int | `90` | Days of state to retain before pruning old rows. Also applied to the in-memory store, though it never survives long enough to matter. |
| persistence.size | string | `"100Mi"` | Requested size. Ignored when `enabled: false`. |
| persistence.storageClass | string | `""` | StorageClass name; empty uses the cluster default. Ignored when `enabled: false`. |
| podSecurityContext | object | `{"fsGroup":65532,"runAsGroup":65532,"runAsNonRoot":true,"runAsUser":65532,"seccompProfile":{"type":"RuntimeDefault"}}` | Pod-level security context. |
| resources | object | `{"limits":{"cpu":"500m","memory":"256Mi"},"requests":{"cpu":"50m","memory":"64Mi"}}` | Compute resources for the container. |
| restartPolicy | string | `"OnFailure"` | Restart policy for the pod: OnFailure or Never (CronJob requirement). |
| rules | list | `[{"maxPriceEUR":5,"minDiscountPercent":0,"name":"cheap-socks","namePattern":"(?i)socks","sizes":[]}]` | Named filter rules. A deal alerts if it matches ANY rule; a rule matches iff every non-empty field inside it matches. |
| schedule | string | `"0 8,20 * * *"` | Cron schedule for the CronJob. Interpreted in `timeZone` when set, otherwise in the kube-controller-manager's local time (UTC on most managed clusters). Default fires at 08:00 and 20:00 local time. |
| securityContext | object | `{"allowPrivilegeEscalation":false,"capabilities":{"drop":["ALL"]},"readOnlyRootFilesystem":true}` | Container-level security context. |
| serviceAccount.annotations | object | `{}` | Extra annotations for the ServiceAccount. |
| serviceAccount.create | bool | `true` | Create a dedicated ServiceAccount. |
| serviceAccount.name | string | `""` | Explicit ServiceAccount name; auto-generated when empty. |
| source | object | `{"baseURL":"https://www.uniqlo.com","clientID":"uq.de.web-spa","gender":"men","kind":"uniqlo","language":"en","maxRetries":3,"region":"de","requestsPerSecond":1,"sizeCodes":["MSC027","SMA004"],"sort":2,"timeout":"15s","userAgent":"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"}` | The source configuration is passed through to the app's YAML. |
| source.baseURL | string | `"https://www.uniqlo.com"` | Base URL of the Uniqlo storefront. |
| source.clientID | string | `"uq.de.web-spa"` | Required x-fr-clientid header value. |
| source.gender | string | `"men"` | Gender segment to filter by. One of: men, women, kids, baby. |
| source.kind | string | `"uniqlo"` | Source kind; only "uniqlo" is supported today. |
| source.language | string | `"en"` | Language path segment (e.g. "en"). |
| source.maxRetries | int | `3` | Max retries on 5xx/429 responses. |
| source.region | string | `"de"` | Region path segment (e.g. "de"). |
| source.requestsPerSecond | int | `1` | HTTP request rate limit. |
| source.sizeCodes | list | `["MSC027","SMA004"]` | Size codes to pre-filter the listing by (URL query parameter). |
| source.sort | int | `2` | Sort parameter forwarded to the listing endpoint. |
| source.timeout | string | `"15s"` | Per-request HTTP timeout. |
| source.userAgent | string | `"Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"` | User-Agent request header. Uniqlo's Akamai front-end rejects obviously-bot-shaped UAs with HTTP/2 INTERNAL_ERROR, so a modern-Chrome UA is required in practice. Bump the Chrome major version yearly. Set to an empty string to fall back to the binary's compiled-in default. |
| successfulJobsHistoryLimit | int | `3` | Number of successful finished jobs to keep. |
| timeZone | string | `"Europe/Berlin"` | IANA time-zone name (e.g. "Europe/Berlin", "America/New_York", "Asia/Tokyo") in which `schedule` is interpreted. Requires Kubernetes 1.27+. When empty, the cluster's default is used. Since this app is Uniqlo-DE-shaped, a European TZ is the sensible default. |
| tolerations | list | `[]` | Tolerations for the pod. |
| ttlSecondsAfterFinished | int | `3600` | Seconds a finished Job is kept before automatic deletion. |

----------------------------------------------
Autogenerated from chart metadata using [helm-docs v1.14.2](https://github.com/norwoodj/helm-docs/releases/v1.14.2)
