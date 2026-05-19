# Downloads-Only Local Path: Implementation Checklist

## Goal

Allow users to browse/select local paths, but strictly limit access to the `~/Downloads` directory subtree.

## Scope

- [ ] Applies to local path input/browse flows in web and desktop surfaces where relevant.
- [ ] Covers both picker-based selection and manual path input/paste.
- [ ] Enforced at both UI layer and backend/API layer.

## Requirements

- [ ] Define canonical allowed root as absolute path to `~/Downloads`.
- [ ] Normalize every incoming path before validation.
- [ ] Resolve symlinks (`realpath`) before final authorization.
- [ ] Reject any path outside allowed root subtree.
- [ ] Return consistent error message for blocked paths (for example: `Only paths inside Downloads are allowed`).

## Backend/API Enforcement (authoritative)

- [ ] Locate all endpoints/handlers that accept local file/folder paths.
- [ ] Add centralized validator utility (single source of truth).
- [ ] Validator must:
  - [ ] Convert to absolute path.
  - [ ] Resolve symlinks.
  - [ ] Compare against resolved Downloads root with safe prefix check.
  - [ ] Handle non-existent paths safely (validate nearest existing parent where needed).
- [ ] Apply validator to every path entry point before file system operations.
- [ ] Add structured logging for denied paths (without leaking sensitive data).

## UI / UX Controls

- [ ] Start file/folder picker at `~/Downloads`.
- [ ] Prevent navigating above allowed root in browse UI when possible.
- [ ] Validate pasted/typed path client-side before submit.
- [ ] Still submit to backend validation (client checks are advisory only).
- [ ] Show clear inline error state and recovery hint.

## Security Edge Cases

- [ ] Block `../` traversal attempts.
- [ ] Block symlink escape from inside Downloads to outside directories.
- [ ] Handle macOS case sensitivity/case insensitivity safely.
- [ ] Handle trailing slashes, duplicate separators, and unicode normalization.
- [ ] Handle hidden files/folders in Downloads according to product policy.

## Testing

- [ ] Unit tests for validator:
  - [ ] Valid path inside Downloads.
  - [ ] Direct outside path.
  - [ ] Traversal path.
  - [ ] Symlink escape path.
  - [ ] Non-existent path case.
- [ ] Integration/API tests:
  - [ ] Endpoint accepts valid Downloads path.
  - [ ] Endpoint rejects outside path with expected status and message.
- [ ] E2E/UI tests:
  - [ ] Picker opens at Downloads.
  - [ ] Manual input outside Downloads is blocked with visible error.

## Rollout

- [ ] Add feature flag if behavior change can affect existing users/workflows.
- [ ] Add release note and migration note (if previously unrestricted).
- [ ] Monitor denied-path metrics after release for false positives.

## Acceptance Criteria

- [ ] No local path operation can access files outside resolved `~/Downloads`.
- [ ] All path entry points are covered by the same backend validator.
- [ ] Tests pass and include traversal/symlink regression coverage.
