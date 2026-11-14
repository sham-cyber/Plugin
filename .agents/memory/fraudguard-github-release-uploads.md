---
name: GitHub release asset uploads
description: Limits of the connected GitHub integration when publishing binary release assets.
---

The connected GitHub API client can create and read releases, but release assets upload through `uploads.github.com`, which the connector proxy may block. GitHub's web UI also requires a signed-in browser session. A release is not complete until its assets are visibly attached and verified.

**Why:** The connector created a draft release but could not upload its binary assets; the browser session required user authentication.

**How to apply:** Before compiling large binaries or creating a release, verify a working asset-upload path. Keep the release in draft until assets are attached and verified, and never request credentials in chat.
