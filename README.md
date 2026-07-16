<img src="./logo.svg" width="40" height="40" alt="0x logo" align="left" />

# 0×/retry

> Retry any command with exponential backoff. One tool, one job.

<br clear="left"/>

[![License: MIT](https://img.shields.io/badge/License-MIT-blue.svg)](./LICENSE)
[![0x Suite](https://img.shields.io/badge/0x-suite-red)](https://github.com/0xProgress)

---

## Why

Shell retry loops suck. You rewrite them constantly, get the backoff wrong, and forget `2>&1`. `retry` handles it so you don't have to.

```bash
# Instead of this:
for i in 1 2 3 4 5; do
  curl https://api.com && break
  sleep $((2**i))
done

# Do this:
retry -t 5 -b exp -- curl https://api.com
```

---

## Install

```bash
curl -sSL https://raw.githubusercontent.com/0xProgress/retry/main/install.sh | sh
```

This clones the repo, builds it with `make`, and installs the binary to `/usr/local/bin` (or `$PREFIX` if set). Requires `git`, `make`, and Go on your `PATH` to build — the resulting binary itself is a single static executable with zero runtime dependencies.

<details>
<summary>Prefer to build manually?</summary>

```bash
git clone https://github.com/0xProgress/retry.git
cd retry
make build
cp retry /usr/local/bin/retry   # or wherever you keep binaries
```

Or, if you just want it on your `$GOPATH/bin`:

```bash
go install github.com/0xProgress/retry@latest
```
</details>

---

## Usage

```bash
# Retry up to 3 times
retry --times 3 -- ping google.com

# Exponential backoff (1s → 2s → 4s → 8s)
retry -t 5 -b exp -- curl -s https://flaky-api.com

# Fixed interval (every 5 seconds)
retry -t 10 -d 5 -- npm publish

# Only retry on specific errors
retry -t 3 --retry-if "Connection refused|timeout" -- ./my-script
```

---

## Options

| Flag | Description | Default |
|------|-------------|---------|
| `-t, --times N` | Max attempts | `5` |
| `-d, --delay N` | Base delay in seconds | `1` |
| `-b, --backoff TYPE` | `linear`, `exp`, or `fixed` | `linear` |
| `--retry-if RE` | Only retry if stderr matches regex | (all failures) |
| `-v, --verbose` | Show attempt timing | off |
| `--no-color` | Disable color output | auto-detected |
| `--version` | Print version | — |
| `-h, --help` | Show help | — |

---

## Exit Codes

| Code | Meaning |
|------|---------|
| `0` | Command succeeded |
| `1` | Failed after all retries |
| `2` | Invalid arguments |
| `3` | Command not found |
| `128+N` | Command was terminated by signal `N` (e.g. `130` = `SIGINT`) |

---

## Real-world examples

**CI pipeline**
```yaml
- name: Deploy
  run: retry -t 3 -b exp -- eb deploy
```

**Database migrations**
```bash
retry -t 10 --retry-if "deadlock|timeout" -- \
  psql -c "ALTER TABLE users ADD COLUMN email_verified BOOLEAN;"
```

**Flaky tests**
```bash
retry -t 3 -v -- go test ./pkg/integration/...
```

---

## Philosophy

Part of the [0× suite](https://github.com/0xProgress) — tools that do one job, have zero bloat, and stay out of your way.

- No config files
- No runtime dependencies (single static binary once built)
- No surprises
- `--help` is all the docs you need

---

## License

Licensed under [MIT](./LICENSE)