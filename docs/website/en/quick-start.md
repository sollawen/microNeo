## Quick Start

**Step 1: One-line install of microNeo**

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/install.sh | sh
```

**Step 2: Detect which AI agents you have**

```bash
microneo --check-agent
```

> You only need to run this once — microNeo will remember which AIs are installed on your machine. Of course, if you install another AI agent later, just run this command again.

**Step 3: Open the file you want to discuss with the AI**

```bash
microneo README.md
```

- Open any file — a Markdown doc, source code, JSON, YAML, or any text file.
- Edit the file like in any other editor; shortcuts are similar to VS Code.
- `Alt-Enter` opens a dialog with the AI at the current cursor; press `Alt-Enter` again to send the message to the AI agent.

---

## Common Hotkeys

| Action | Shortcut | Action | Shortcut | Action | Shortcut |
| --- | --- | --- | --- | --- | --- |
| Save | Ctrl-S | Undo | Ctrl-Z | Copy | Ctrl-C |
| Quit | Ctrl-Q | Search | Ctrl-F | Cut | Ctrl-X |
| Select | Shift-Up/Down | Send message to AI | Alt-Enter | Paste | Ctrl-V |
| Command Mode | Ctrl-E | Help | Ctrl-G |

- Most shortcuts are the same as in VS Code.
- Press `Ctrl-G` to see more shortcuts and commands.

---

## Set as Default Editor

```bash
export EDITOR=microneo
```

- Works seamlessly with `Claude Code`, `Yazi`, and other tools that respect `$EDITOR`.
- microNeo is small and fast, making it a great default editor for these tools.
