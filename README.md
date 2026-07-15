# Temporal Transactional Shell Environment (TTSE)

This repository contains a minimal TTSE implementation for a Bash environment.

## What is included

- A Go-based controller that creates a temporary overlay filesystem sandbox under /tmp/ada_wong_sandbox.
- A Bash wrapper named ttse that exposes start, status, commit, and undo actions.
- Graceful handling for environments without overlay mount privileges.

## Build and use

```bash
cd /workspaces/ADA-Wong-2
GOOS=linux GOARCH=amd64 go build -o ttse-controller ./main.go
chmod +x ttse ttse-controller
./ttse start
./ttse status
./ttse commit
./ttse undo
./ttse version
```

## Notes

- The implementation uses native Linux overlay mounting and is intentionally conservative.
- CRIU-specific checkpoint/restore orchestration is documented at the code level but is not executed automatically in this container because the required binary is unavailable.
- No terminal theme or shell appearance was modified.
