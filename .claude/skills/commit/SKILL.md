---
name: commit
description: Create a git commit with proper formatting and co-authoring
---

# Commit Skill

Create a properly formatted git commit with automatic co-author attribution.

## Usage

When the user asks you to commit changes, follow these steps:

1. **Run tests and linting**
   - Run `git diff --staged --name-only` (and check unstaged changes too) to see which files changed
   - If every changed file is non-code and non-configuration — e.g. docs (`*.md`), comments-only skill/prompt files, images, or other pure content/asset files — skip this step entirely and say so
   - Otherwise, detect the project's test and lint commands (e.g. from `package.json` scripts, a `Makefile`, or existing CI config) and run them
   - If both a test suite and a linter exist, run both
   - If any changed file is Go code (or a `go.mod`/`go.sum` in the repo), also run `go mod tidy` first, then `git diff --exit-code go.mod go.sum` to confirm it produced no changes
     - If `go mod tidy` changes `go.mod`/`go.sum`, stage those changes as part of the commit (don't discard them)
   - If any changed file is Go code (or a `go.mod`/`go.sum` in the repo), also run `go vet ./...`
   - If any of the above fails, **stop immediately** — do not create the commit. Report the failure output to the user and either fix the underlying issue (then re-run) or wait for guidance. Never commit with failing tests, lint errors, or `go vet` errors, and never bypass this with `--no-verify` or by skipping the checks
   - If the project has no discoverable test or lint tooling, note that and proceed

2. **Review staged changes**
   - Run `git status` to see staged and unstaged changes
   - Run `git diff --staged` to review what will be committed

3. **Analyze the changes**
   - Understand what was changed and why
   - Determine the commit type (feature, fix, refactor, docs, test, etc.)
   - Draft a concise, clear commit message

4. **Commit with proper formatting**
   - Create commits with messages that follow conventional commits when appropriate
   - Always end the commit message with the co-author line using the **specific model name**:
     ```
     Co-Authored-By: Claude Haiku 4.5 <noreply@anthropic.com>
     ```
     **Note:** Example uses `Claude Haiku 4.5` but real commits should use whatever the current model is
   - Use a HEREDOC to pass the commit message to ensure proper formatting

5. **Verify success**
   - Run `git status` after the commit to confirm it succeeded
   - If a pre-commit hook fails, fix the underlying issue and create a NEW commit (do not amend)

6. **Push**
   - Push the current branch to its remote (e.g. `git push`, or `git push -u origin <branch>` if it has no upstream yet)
   - If the push is rejected because the remote has new commits, pull/rebase and resolve before retrying — never force-push without explicit user confirmation

## Example

```bash
git commit -m "$(cat <<'EOF'
Fix race condition in chat message handling.

The chat processor was not properly synchronizing access to the message buffer,
causing dropped messages under concurrent load. Added mutex protection.

Co-Authored-By: Claude Haiku 4.5 <noreply@anthropic.com>
EOF
)"
```

## Important Notes

- Only commit when explicitly asked by the user
- Tests and linting must pass before a commit is created — failing checks block the commit until fixed or the user says otherwise
- Skip tests/linting only when the changes are purely non-code/non-configuration (docs, comments, assets, etc.); any code or config change requires running them
- For Go changes: run `go mod tidy` before checking `go.mod`/`go.sum` are clean, and run `go vet ./...` — both must pass before committing
- Never commit sensitive files (.env, credentials, secrets)
- Prefer creating new commits over amending existing ones
- Stage specific files by name rather than using `git add -A` or `git add .`
- Review what's included after staging to catch sensitive files
- Never force-push (`--force`/`--force-with-lease`) without explicit user confirmation
