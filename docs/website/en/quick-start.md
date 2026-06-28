
## Quick Start

**Step 1: One-line install of microNeo**

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/install.sh | sh
```

**Step 2: Detect which AI agents you have**

```bash
microneo --check-agent
```
You only need to do this once — microNeo will remember which AIs are installed on your machine. Of course, if you install another AI agent later, just run this command again.

**Step 3: Open the file you want to discuss with the AI**

```bash
# Open any file — a Markdown doc or a source file
microneo README.md
```

### Set as Default Editor

```bash
export EDITOR=microneo
```

- Works seamlessly with `Claude Code`, `Yazi`, and other tools that respect `$EDITOR`.
- microNeo is small and fast, making it a great default editor for these tools.

---

## Hotkeys

| Action        | Shortcut   | Action        | Shortcut   |
| ------------- | ---------- | ------------- | ---------- |
| Save          | `Ctrl-S`   | Undo          | `Ctrl-Z`   |
| Quit          | `Ctrl-Q`   | Search        | `Ctrl-F`   |
| Command Mode  | `Ctrl-E`   |               |            |

Press `Ctrl-E` and type `help` for more commands.
