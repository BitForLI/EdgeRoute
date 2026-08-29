# Verification record

Last updated: 2026-08-29 (Australia/Sydney).

## Passed locally

- `go test ./...`
- `go vet ./...`
- Linux/amd64 `go test -race ./...` in `golang:1.25.0-bookworm`
- `FuzzBuildRoutingKeyStable`, 10 seconds, 696,984 executions
- `FuzzWeightedRendezvousNeverSelectsInvalidCandidate`, 10 seconds, 3,910,262 executions
- PowerShell parser checks for the experiment runner, HLS verifier, e2e runner, and demo script
- GNU Make dry-run expansion for all documented targets
- Quality Controller `linux/amd64` image build with a 94 KiB Docker context after excluding experiment evidence
- Day 6 three-policy smoke matrix: 36/36 runs accepted by the strict processor
- Repeated Day 6 processing: identical SHA-256 for `runs.csv`, `summary.csv`, `report.md`, and `policy-comparison.png`
- Post-experiment cluster check: CoreDNS 2/2 Ready, Quality Controller 1/1 Ready, all three edge Deployments Ready, Corefile restored to `routingmode adaptive`

## Pending after the requested upload-first checkpoint

- Rebuild the patched CoreDNS image from the final documentation/automation commit.
- Apply the final manifests with their declared `:dev` images.
- Run `scripts/e2e-smoke.ps1` end to end against kind.
- Run `scripts/demo.ps1` once and inspect its captured NodeQuality stages.
- Confirm the GitHub CI result for the uploaded commit.
- Create and push release tag `v0.1.0` only after every item above passes.

An attempted final manifest apply exposed server-side field-manager conflicts because the Day 6 runner had temporarily patched the Corefile and image. `make deploy` now uses `--force-conflicts` for the repository-owned `deploy/` fields so the committed lab state deliberately takes ownership back. The final apply/e2e verification is pending; this record does not present that fix as already tested.
