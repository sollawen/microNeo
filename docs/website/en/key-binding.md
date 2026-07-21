
All keybindings in microNeo are defined in a single JSON file that users can freely customize.

### File Location

```
~/.config/microNeo/bindings.json
```

Use the `--config <dir>` flag at startup to temporarily override the configuration directory.

### File Structure

The top level of `bindings.json` is an object where each key corresponds to a pane type (editor region). microNeo reads them at startup in this order:

1. **Default values** for each section (built into microNeo)
2. **User-defined** configurations from `bindings.json`

**Later entries override earlier ones.** So if the default is `Ctrl-S` → `Save` and the user writes `"Ctrl-S": "Quit"` in the file, `Ctrl-S` becomes `Quit`.

### Four Sections

| Section | Purpose |
|---|---|
| `buffer` | Main editor (editing files, writing code) |
| `command` | Command-line mode (enter with `Ctrl-E`, type commands after `:`) |
| `notepane` | notePane (the input box for chatting with AI) |
| `terminal` | Terminal panel |

### Top-Level vs Section Syntax

Both syntaxes work:

```json
{
    "Ctrl-S": "Save"
}
```

is equivalent to:

```json
{
    "buffer": {
        "Ctrl-S": "Save"
    }
}
```

Top-level keys default to the buffer section. **To configure exclusive actions for notePane or command, you must use section syntax** — otherwise the action will be rejected (the top-level key is treated as belonging to buffer section).

### Common Examples

#### 1. Change the notePane send key

The default `Alt-Enter` is the send key. To switch it to `F5`:

```json
{
    "buffer": {
        "F5": "NotePaneOpen"
    },
    "notepane": {
        "F5": "NotePaneSend"
    }
}
```

- **buffer section**: `F5` opens notePane from the main editor
- **notepane section**: `F5` sends the draft from inside notePane

The two sections are **independent** — you must write two lines to remap a key.

#### 2. Disable a key

To disable `Alt-Enter` (make it a no-op):

```json
{
    "buffer": {
        "Alt-Enter": "None"
    },
    "notepane": {
        "Alt-Enter": "None"
    }
}
```

- `None` is a special action in microNeo meaning "do nothing". This is the standard way to remap a key: first disable the old key, then bind the new key.
- Like vim and other editors, microNeo supports **multiple keys mapping to the same action** (e.g. `Ctrl-H`, `Backspace`, `Shift-Backspace` all map to `Backspace`). So if you want your newly defined key to override a built-in key, you must explicitly set the built-in key to `"None"`.

#### 3. Change common editing keys

notePane automatically follows the main editor's common editing key configuration:

```json
{
    "buffer": {
        "Enter":   "None",
        "Alt-a":   "StartOfLine",
        "Tab":     "InsertTab"
    }
}
```

- `Enter` is disabled in notePane
- `Alt-a` jumps to the beginning of the line in notePane as well (default is `StartOfText`)
- `Tab` follows the composite chain

#### 4. notePane's Esc cannot be remapped

`Esc` is hard-bound to `NotePaneClose` in notePane and **cannot** be overridden by the user's `Esc: None`. This is a microNeo safety mechanism to prevent users from trapping themselves inside notePane with no way out.


### Full Action List

To see all available actions, run:

```
:command help
```

Or refer to the upstream micro documentation: [micro Bindings](https://github.com/zyedidia/micro/blob/master/runtime/help/bindings.md).

### FAQ

#### My bindings.json reports an error. What should I do?

Check the JSON format:
- Trailing commas at the end
- Quotes should be plain ASCII double quotes
- Key names should be in `"Alt-Enter"` format (not `<Alt-Enter>`)

You can validate the format with [JSONLint](https://jsonlint.com/).


#### My changes to bindings.json don't take effect?

1. Check that the file path is correct (`~/.config/microNeo/bindings.json`, not the upstream micro location)
2. Check that the JSON format is valid
3. Try restarting microNeo
