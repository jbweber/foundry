# GitHub Actions Workflows

CI/CD automation for Foundry.

## Workflows

### [ci.yml](ci.yml) - Continuous Integration

**Trigger**: Every push and pull request

**What it does**: Runs `make check` (formatting, linting, tests, race detection)

**Why**: Fast feedback on code quality before merge

---

### [release.yml](release.yml) - Release Automation

**Trigger**: Version tags (`v*.*.*`)

**What it does**:
1. Run full test suite (`make check`)
2. Build Linux amd64 binary (static, no CGO)
3. Create tarball with docs
4. Build Docker image (Alpine + binary)
5. Generate SBOM attestation
6. Push to `ghcr.io/jbweber/foundry`
7. Create GitHub release with changelog

**Why Docker Buildx?**
Enables attestations (SBOM for supply chain security). Without it, the build fails with: `Attestation is not supported for the docker driver`

## Artifacts

- **Binary tarball**: `foundry_X.Y.Z_linux_amd64.tar.gz` (static binary + docs)
- **Docker image**: `ghcr.io/jbweber/foundry:X.Y.Z` and `:latest` (Alpine + binary)
- **Checksums**: `checksums.txt` (SHA256 hashes)
- **SBOM**: Attestation attached to Docker image (supply chain security)

## Key Decisions

**Two workflows**: Fast CI on every commit, slow release only on tags

**GoReleaser**: Industry standard, handles builds/Docker/changelog/checksums automatically

**Static binaries**: CGO disabled, works on any Linux distro, single file deployment

**Attestations**: Required for supply chain security, needs Buildx to generate

## Maintenance

**Update Go version**: Change in both workflows + `go.mod`

**Test locally**: `make check` (CI) or `goreleaser release --snapshot --clean` (release build)

**Debug failed releases**: Check Actions logs, common issues are test failures or GHCR permissions
