microNeo's most commonly used command-line flags, for configuration maintenance and AI agent checks/updates without opening the editor.


### `reset-settings` — Reset configuration

```bash
microneo --reset-settings
```

Restore `settings.json` in the user config directory to microNeo's built-in defaults.
- The existing `settings.json` is first backed up to `settings.json.backup`, then overwritten with the new file
- Useful when the config has been messed up, or after an upgrade when fields no longer line up


### `check-agent` — Check and fix AI agent extensions

```bash
microneo --check-agent
```

microNeo talks to AI agents (pi, opencode, claude cli) by installing a tiny aibp communication-protocol extension on the agent side.

This command:
- Scans the machine to see which AI agents are installed
- For each agent, checks whether the aibp extension is in place and whether the version is current
- Installs missing ones, upgrades outdated ones
- Exits immediately when done — never opens the editor

**When to run it:**

- After installing microNeo for the first time
- After installing a new AI agent
- When communication with an agent fails, or sent messages get no response

---

### `update-aibp` — Upgrade AI agent extensions only

```bash
microneo --update-aibp
```

Does only one thing: upgrade the aibp extension to the latest version. No other checks.

- Agents like pi self-update their aibp — you don't need this command for them
- opencode and claude cli don't self-update aibp, so you'll need to run this manually
- Exits immediately when done

I may merge this command with the previous one in the future.