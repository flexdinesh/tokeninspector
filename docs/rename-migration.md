# Migration Guide: tokeninspector → tokeninsights

This project has been renamed from **tokeninspector** to **tokeninsights**. This guide covers everything you need to do manually to continue using the plugins and CLI.

---

## 1. Environment Variables (REQUIRED)

The environment variable prefixes have changed from `TOKENINSPECTOR_` to `TOKENINSIGHTS_`.

| Old | New |
|---|---|
| `TOKENINSPECTOR_DB_PATH` | `TOKENINSIGHTS_DB_PATH` |
| `TOKENINSPECTOR_RETENTION_DAYS` | `TOKENINSIGHTS_RETENTION_DAYS` |

**Action:** Update your shell profile (`.bashrc`, `.zshrc`, etc.) or any scripts that set these variables.

```bash
# Before (OLD — no longer works)
export TOKENINSPECTOR_DB_PATH=/path/to/custom.db
export TOKENINSPECTOR_RETENTION_DAYS=90

# After (NEW)
export TOKENINSIGHTS_DB_PATH=/path/to/custom.db
export TOKENINSIGHTS_RETENTION_DAYS=90
```

---

## 2. CLI Binary (REQUIRED)

The CLI binary name has changed from `tokeninspector-cli` to `tokeninsights-cli`.

**Action:** Update any shell aliases, scripts, or documentation that reference the old binary name.

```bash
# Before (OLD)
tokeninspector-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --today

# After (NEW)
tokeninsights-cli --db-path ~/.local/state/tokeninsights/tokeninsights.sqlite --today
```

If you had the old binary in your PATH, rebuild from source:

```bash
cd cli
go build -o tokeninsights-cli ./cmd/tokeninsights-cli
```

---

## 3. Database File Migration (REQUIRED — existing data)

The default database file name and state directory have changed:

| Old | New |
|---|---|
| `~/.local/state/tokeninspector/tokeninspector.sqlite` | `~/.local/state/tokeninsights/tokeninsights.sqlite` |
| `$XDG_STATE_HOME/tokeninspector/tokeninspector.sqlite` | `$XDG_STATE_HOME/tokeninsights/tokeninsights.sqlite` |
| `$PWD/.tokeninspector-state/tokeninspector.sqlite` | `$PWD/.tokeninsights-state/tokeninsights.sqlite` |

**Action:** Migrate your existing database file to the new location:

```bash
# Create the new state directory
mkdir -p ~/.local/state/tokeninsights

# Move the existing database
mv ~/.local/state/tokeninspector/tokeninspector.sqlite \
   ~/.local/state/tokeninsights/tokeninsights.sqlite

# Optional: remove the old empty directory
rmdir ~/.local/state/tokeninspector 2>/dev/null || true
```

If you use a custom DB path via `TOKENINSIGHTS_DB_PATH` (or `TOKENINSPECTOR_DB_PATH` before the rename), you do not need to move the file — just update the environment variable (see §1).

---

## 4. OpenCode Plugin Configuration (REQUIRED for OpenCode users)

### 4a. Remove the old plugin entry

Remove any `oc-tokeninspector` entry from your `~/.config/opencode/opencode.jsonc`.

### 4b. Recreate plugin symlinks

The plugin file names have changed:

| Old symlink target | New symlink target |
|---|---|
| `plugins/opencode-server/oc-tokeninspector-server.ts` | `plugins/opencode-server/oc-tokeninsights-server.ts` |
| `plugins/opencode-tui/oc-tokeninspector.tsx` | `plugins/opencode-tui/oc-tokeninsights.tsx` |

**Action:** Recreate the symlinks in `~/.config/opencode/plugins/`:

```bash
# Remove old symlinks
rm -f ~/.config/opencode/plugins/oc-tokeninspector-server.ts
rm -f ~/.config/opencode/plugins/oc-tokeninspector.tsx

# Create new symlinks (adjust $PWD to your clone path)
ln -s "$PWD/plugins/opencode-server/oc-tokeninsights-server.ts" \
   ~/.config/opencode/plugins/
ln -s "$PWD/plugins/opencode-tui/oc-tokeninsights.tsx" \
   ~/.config/opencode/plugins/
```

### 4c. Update `opencode.jsonc`

Add the new plugin entries. The plugin ID has changed from `oc-tokeninspector` to `oc-tokeninsights`:

```jsonc
{
  "plugins": {
    "server": [
      "/home/dee/workspace/tokeninsights/plugins/opencode-server/oc-tokeninsights-server.ts"
    ],
    "tui": [
      "/home/dee/workspace/tokeninsights/plugins/opencode-tui/oc-tokeninsights.tsx"
    ]
  }
}
```

> **Do not** add `plugins/shared/oc-tokeninsights-writer.ts` or `plugins/shared/writer-client.ts` to the config. The writer is a worker module loaded internally by the server plugin.

---

## 5. Pi Extension (REQUIRED for Pi users)

The Pi extension package name and symlink path have changed:

| Old | New |
|---|---|
| `pi-tokeninspector` | `pi-tokeninsights` |

**Action:** Recreate the extension symlink:

```bash
# Remove old symlink
rm -f ~/.pi/agent/extensions/pi-tokeninspector

# Create new symlink (adjust $PWD to your clone path)
ln -s "$PWD/plugins/pi" ~/.pi/agent/extensions/pi-tokeninsights

# Rebuild the extension
cd ~/.pi/agent/extensions/pi-tokeninsights
npm install   # or bun install
```

---

## 6. Git Remote (REQUIRED for contributors)

If you push to the GitHub repository, update the remote URL after the repository is renamed on GitHub:

```bash
git remote set-url origin git@github.com:flexdinesh/tokeninsights.git
```

> This step requires the repository owner to rename the GitHub repository first.

---

## Quick Checklist

- [ ] Rename environment variables (`TOKENINSPECTOR_*` → `TOKENINSIGHTS_*`)
- [ ] Update CLI binary references (`tokeninspector-cli` → `tokeninsights-cli`)
- [ ] Move database file to new location (if using default path)
- [ ] Recreate OpenCode plugin symlinks with new file names
- [ ] Update `opencode.jsonc` with new plugin ID (`oc-tokeninsights`)
- [ ] Recreate Pi extension symlink (`pi-tokeninsights`)
- [ ] Update git remote URL (after GitHub repo rename)
- [ ] Rebuild CLI from source: `cd cli && go build -o tokeninsights-cli ./cmd/tokeninsights-cli`
