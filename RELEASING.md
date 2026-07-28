# Releasing

Versions are shared: the git tag drives the GitHub release, PyPI and npm.

## One-time setup

Two things need credentials, and both are yours to create — they cannot be
scripted from here.

1. **PyPI trusted publishing** (no token to store). Go to
   <https://pypi.org/manage/account/publishing/> and add a pending publisher:

   | Field | Value |
   |---|---|
   | PyPI project name | `xtxt` |
   | Owner | `SaiMouli3` |
   | Repository | `xtxt` |
   | Workflow | `release.yml` |
   | Environment | `pypi` |

   Then create the `pypi` environment under repo Settings → Environments.

2. **npm token.** `npm token create --read-only=false`, then add it as the
   repository secret `NPM_TOKEN` (Settings → Secrets → Actions). Until that
   secret exists the npm job skips itself rather than failing the release.

   The npm package is **`xtxt-js`**, not `xtxt`: the bare name is held by an
   abandoned v0.0.0 placeholder published in 2022. PyPI and the Go module use
   plain `xtxt`. If npm ever releases the name, that is a rename, not a
   silent retag.

Go needs nothing: the module proxy picks up tags automatically.

## Cutting a release

```sh
# 1. bump the version in both SDKs
vim sdk/python/pyproject.toml sdk/js/package.json   # version = X.Y.Z

# 2. confirm all three implementations agree
go test ./... && (cd sdk/python && python -m pytest -q) && (cd sdk/js && node --test)

# 3. tag and push
git commit -am "Release vX.Y.Z"
git tag vX.Y.Z
git push origin main --tags
```

The `release` workflow re-runs CI, then publishes the GitHub release, the PyPI
wheel and the npm package. A failed test blocks all three.

## Versioning

The **specification** and the **implementations** version separately. A spec
change that adds a directive is a minor bump; one that removes or repurposes
one is major (SPEC §7). An implementation may ship many versions against one
spec version.
