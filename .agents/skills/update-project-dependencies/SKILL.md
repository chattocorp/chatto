---
name: update-project-dependencies
description: Update Chatto Go and frontend dependencies within their compatible version ranges, then run tests.
---

# Update Project Dependencies

1. Record direct dependency versions from `cli/go.mod` and
   `apps/frontend/package.json`. Keep the update within the requested product
   and module scope.
2. In `cli/`, run:

   ```sh
   mise x -- go get -u ./...
   mise x -- go mod tidy
   ```

3. In `apps/frontend/`, run `mise x -- pnpm update`.
4. Review manifest and lockfile changes. Do not change major versions or widen
   declared version ranges without a request to do so. Version ranges do not
   prove behavioral compatibility.
5. Run `mise test` from the repository root. If tests fail, stop and report the
   failures. Leave the changes for review; do not roll them back automatically.
6. Report changed direct dependency versions, relevant behavior changes, and
   test results. State any modules or checks that were outside the scope.
