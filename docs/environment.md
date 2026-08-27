# Environment and baseline

## Pinned toolchain

- Go 1.25.0, matching `go.mod`
- CoreDNS 1.14.2, matching `Makefile`
- Windows amd64 local host
- Upstream commits listed in `UPSTREAM.md`

The local Go archive came from the official Go download site and was verified against SHA-256:

```text
go1.25.0.windows-amd64.zip
89efb4f9b30812eee083cc1770fdd2913c14d301064f6454851428f9707d190b
```

The portable toolchain lives under `.tools/` and is intentionally ignored by Git.

## Reproduce the upstream baseline

PowerShell:

```powershell
New-Item -ItemType Directory -Force .tmp/gomodcache,.tmp/gocache,.tmp/temp | Out-Null
$env:GOMODCACHE = (Resolve-Path .tmp/gomodcache).Path
$env:GOCACHE = (Resolve-Path .tmp/gocache).Path
$env:TEMP = (Resolve-Path .tmp/temp).Path
$env:TMP = $env:TEMP
& ./.tools/go/bin/go.exe test ./...
& ./.tools/go/bin/go.exe build ./...
```

Observed at the pinned upstream commit:

```text
go test ./...  exit 0; no upstream test files
go build ./... exit 0
```

## Missing local infrastructure

At baseline time:

- Docker CLI was installed, but the Docker service was not running;
- `kubectl` was available through Docker Desktop;
- `kind`, `helm`, and `make` were not installed.

CoreDNS image build, kind cluster creation, CRD installation, and HLS deployment are therefore not yet claimed as verified.
