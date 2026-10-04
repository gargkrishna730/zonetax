# ZoneTax end-to-end QA plan

Run against a real 3-AZ EKS test cluster. Each test has an ID, steps and a pass criterion.
Results go in `docs/qa/report-<date>.md`.

| ID | Area | Test | Pass criterion |
|---|---|---|---|
| A1 | Build | `go test -race ./...`, `go vet`, gofmt | all pass |
| A2 | Build | UI lint, tsc, vitest, build | 0 errors, all tests pass |
| A3 | Build | helm lint (default + persistence), manifest drift | pass, no diff |
| B1 | Release | Helm repo lists every tag; chart images pinned to tag | versions present, `image: ...:<tag>` |
| B2 | Release | Agent + collector images exist for amd64 and arm64 per tag | both arches |
| B3 | Release | CLI tarballs match checksums.txt and run | sha256 OK, prints version |
| B4 | Release | README links (accuracy doc, releases, install.yaml) resolve | HTTP 200 |
| C1 | Install | `helm install` dry-run (server) in a fresh namespace | accepted by API server |
| C2 | Install | `kubectl apply --dry-run=server -f install.yaml` | accepted |
| C3 | Install | Fresh collector (published image) cold start in a scratch namespace | /healthz ok; first data within 2 scrapes; empty states honest (has_data=false before data) |
| D1 | Upgrade | Upgrade test install to published chart (pinned images) | rollout ok, history restored, no data loss |
| D2 | Upgrade | `helm rollback` | previous revision healthy |
| E1 | Accuracy | Send a known volume cross-AZ between two test pods | ZoneTax attributes the route within 5% |
| E2 | Accuracy | Same volume same-AZ | not billed (no cross-AZ entry), counted as same-AZ |
| E3 | Accuracy | Cost = GB x effective price | exact |
| F1 | API | /healthz, /costs, /top, /history (all ranges), /map (all ranges + custom + invalid) | correct status codes and shapes |
| F2 | API | Invariants: map total = sum(entries); entries sorted; history buckets sum ~= map total | hold |
| G1 | Resilience | Collector restart | history restored, gap marked, no spike |
| G2 | Resilience | Agent pod restart | no cost spike in the next buckets |
| H1 | UI | Load, every range, both views, filters, panels, theme | works, no console errors |
| H2 | UI | KPI numbers == API numbers | match |
| H3 | UI | axe WCAG 2.2 AA dark + light, keyboard walkthrough | 0 violations, all reachable |
| I1 | CLI | top/report all ranges, -n, --json, bad range, unreachable | correct output and exit codes |
| J1 | Footprint | Agent and collector CPU/memory under load | within requests/limits |
| J2 | Security | RBAC is read-only (get/list/watch pods, nodes) | no write verbs |
