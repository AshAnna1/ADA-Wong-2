# TTSE User Guide

TTSE is a lightweight transactional shell environment for creating isolated overlay-based workspaces and tracking transaction history.

## What the tool does

- Creates a temporary overlay sandbox for the current working directory
- Tracks transaction steps and history entries
- Supports start, status, commit, undo, and version commands
- Stores snapshots and history metadata under the sandbox directory

## Commands

- `ttse start` - Create a new sandbox transaction for the current directory
- `ttse status` - Show the current transaction state and history
- `ttse commit` - Record a commit snapshot and clean up the sandbox
- `ttse undo` - Record an undo snapshot and reset the transaction state
- `ttse version` - Show the installed tool version
- `ttse docs` - Show this user guide and usage information

## Usage examples

```bash
./ttse start
./ttse status
./ttse commit
./ttse undo
./ttse version
./ttse docs
```

## Notes

- The tool uses overlay mounts when the environment supports them.
- If overlay mount capabilities are unavailable, the tool reports that condition clearly.
- The commit and undo actions record snapshots under the sandbox metadata directory for review.
