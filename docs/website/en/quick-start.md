## Quick Start

**Step 1: One-line install of microNeo**

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/tools/install.sh | sh
```

> Works perfectly on Linux/Mac. Windows requires full shell support and hasn't been tested.

If you hit `raw.githubusercontent.com` being rate-limited (HTTP 429) or unreachable, you can use this jsDelivr mirror instead:

```bash
curl -fsSL https://cdn.jsdelivr.net/gh/sollawen/microNeo@master/tools/install.sh | sh
```

**Step 2: Detect which AI agents you have**

```bash
microneo --check-agent
```

- You only need to run this once — microNeo will remember which AIs are installed on your machine.
- Of course, if you install another AI agent later, just run this command again.
- Currently supports OpenCode, Pi, and Claude. However, since Claude is not open source, it works but isn't perfect.

**Step 3: Start using microNeo**

```bash
# Run microNeo in the current directory.
# microNeo has a fully-featured FileManager for easy navigation through the directory tree.

microneo

# Open a specific file with microNeo
microneo README.md
```

- Open any file — a Markdown doc, source code, JSON, YAML, or any text file.
- Edit the file like in any other editor; shortcuts are similar to VS Code.
- `Alt-Enter` opens a dialog with the AI at the current cursor; press `Alt-Enter` again to send the message to the AI agent.
- If you're running multiple AI agents at the same time, press `Alt-I` in the message box to pick the one you want to talk to.

---

## Common Operations While Editing

### Common Hotkeys

| Action | Shortcut | Action | Shortcut | Action | Shortcut |
| --- | --- | --- | --- | --- | --- |
| Save | Ctrl-S | Undo | Ctrl-Z | Copy | Ctrl-C |
| Quit | Ctrl-Q | Search | Ctrl-F | Cut | Ctrl-X |
| Select | Shift-Up/Down | Send message to AI | Alt-Enter | Paste | Ctrl-V |
| Command Mode | Ctrl-E | Help | Ctrl-G |

- Most shortcuts are the same as in VS Code.
- Press `Ctrl-G` to see more shortcuts and commands.

### Talking to an AI agent

1. Open a Markdown document with microNeo and select the part of the text you want to comment on.
1. Press `Alt-Enter` to open the input box and write your thoughts.
1. When ready, press `Alt-Enter` again to send it. The AI will receive your comment.

### Talking to multiple AI agents

If you run multiple AI agents at the same time (e.g., one `opencode` and two `pi`), each one picks a name for itself at startup.

- Default names are `Alpha, Bravo, Charlie...`
- If you don't like the defaults, edit `~/.config/aibp/aibp-names.json` in your config directory and put any names you like, e.g. `Tony, Bruce, Peter, Carol`.

When you press `Alt-Enter` in the editor and more than one AI is present, microNeo automatically opens a menu to let you pick which AI to talk to.

- Your selection is remembered by microNeo. The next time you press `Alt-Enter`, it'll automatically use the same AI.
- To switch to a different AI mid-session, while typing in the dialog, press `Alt-i` and pick a different AI from the menu.

### Switching Color Theme

In microNeo, press `Ctrl-E` to open the command line at the bottom, type `:theme`, and hit Enter — a menu will appear letting you pick a different theme.

### Opening a New File

In microNeo, press `Ctrl-O` to pick a new file.

- `Up/Down` arrows — move the cursor
- `Right arrow` — enter the subdirectory
- `Left arrow` — go to the parent directory
- `Enter` — open the file

---

## Set as Default Editor

- microNeo is small and fast, making it a great default editor for these tools.
- Works seamlessly with `Claude Code`, `Yazi`, and other tools that respect `$EDITOR`.
- Since `microNeo` is a long name, it's recommended to set an alias in your `.zshrc` or `.bashrc` for easier typing.

```bash
export EDITOR=microneo
alias edit='microneo'
```
