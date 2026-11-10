---
name: Bifrost monorepo build
description: Exact source-checkout builds may need local sibling-module replacements instead of module proxy releases.
---

When validating a Bifrost source checkout, the `transports` module may import packages already present in sibling `framework` or plugin directories but not yet present in their published module versions. Use local `replace` directives for those sibling modules to compile against the checked-out source; otherwise Go may resolve an older published module and report a missing package.

**Why:** The inspected dev source contained a package that was absent from the matching published framework module.

**How to apply:** Use the integration helper or equivalent local replacements, verify the intended source revision, and keep the overlay local unless upstream explicitly accepts it.
