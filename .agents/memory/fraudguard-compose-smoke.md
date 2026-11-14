---
name: FraudGuard Compose smoke checks
description: Docker health-check behavior and host-side probes in the managed Replit environment.
---

**Rule:** In Replit's managed Docker environment, Compose health checks may fail at the OCI exec layer even while services are running. For smoke tests, disable container health checks only in a temporary override, publish dependencies on loopback, and probe Redis over raw TCP and the mock provider over HTTP.

**Why:** Redis and the mock provider started, but Docker could not execute their in-container health-check commands. Host-side probes verified service readiness without weakening the base Compose health checks.

**How to apply:** Keep normal health checks in the main Compose file. In the Replit smoke runner, use a generated temporary override; check Redis on TCP port 6379, the provider's `/healthz`, and Bifrost's `/health` from the host, then remove the override and containers.
