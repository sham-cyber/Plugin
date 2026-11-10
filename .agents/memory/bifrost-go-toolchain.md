---
name: Bifrost Go toolchain
description: Go toolchain and formatter behavior when validating the pinned Bifrost source in this Replit environment.
---

The current Replit shell exposes Go 1.26.8 while the pinned Bifrost plugin modules declare Go 1.27.3. The toolchain can be selected from a module with `GOSUMDB=sum.golang.org go <command>`; `gofmt` may not be on `PATH`, so call the formatter at `$(go env GOROOT)/bin/gofmt`.

**Why:** Replit's default `GOSUMDB=off` prevents Go from verifying and downloading the newer toolchain automatically.

**How to apply:** When validating Bifrost modules that declare Go 1.27.3, set `GOSUMDB=sum.golang.org` for Go commands and resolve `gofmt` from the selected toolchain's `GOROOT`. Go subprocesses launched by the Node API must also override `GOSUMDB=off` explicitly so toolchain verification can succeed.
