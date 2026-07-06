## Quick Start

**Step 1: One-line install of microNeo**

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/tools/install.sh | sh
```

> Works perfectly on Linux/Mac. Windows requires full shell support and hasn't been tested.

**Step 2: Detect which AI agents you have**

```bash
microneo --check-agent
```

- You only need to run this once — microNeo will remember which AIs are installed on your machine.
- Of course, if you install another AI agent later, just run this command again.
- Currently supports OpenCode, Pi, and Claude. However, since Claude is not open source, it works but isn't perfect.

**Step 3: Open the file you want to discuss with the AI**

```bash
microneo README.md
```

- Open any file — a Markdown doc, source code, JSON, YAML, or any text file.
- Edit the file like in any other editor; shortcuts are similar to VS Code.
- `Alt-Enter` opens a dialog with the AI at the current cursor; press `Alt-Enter` again to send the message to the AI agent.
- If you're running multiple AI agents at the same time, press `Alt-I` in the message box to pick the one you want to talk to.

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

- microNeo is small and fast, making it a great default editor for these tools.
- Works seamlessly with `Claude Code`, `Yazi`, and other tools that respect `$EDITOR`.
- Since `microNeo` is a long name, it's recommended to set an alias in your `.zshrc` or `.bashrc` for easier typing.

```bash
export EDITOR=microneo
alias edit='microneo'
```
