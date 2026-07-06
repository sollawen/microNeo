# MicroNeo default settings (JSON5)

## Location
microNeo config file: `~/.config/microNeo/settings.json`

When needed, run `microneo --reset-settings` to regenerate a complete `settings.json` with all default values in your user config directory (`~/.config/microNeo`), giving you a clean baseline to customize.

```bash
# bash
# This generates a complete ~/.config/microNeo/settings.json

microneo --reset-settings
```

## Most commonly used options

### Basic
```json
    // ── theme ──
    "colorscheme": "s-dark",		// Dark Mode theme I wrote myself  
    "mouse": true,  				// mouse support.
```

`s-dark` and `s-light` are themes I wrote myself — one light palette, one dark palette.

### Main UI
```json
	// UI settings    
    "statusline": true,  			// display the status line at the bottom of the screen.
    "status-separator": "\ue0b0",  	// triangular separator in the status bar, requires Nerd Font. Fallback: "|"

    "ruler": true,  			// display line numbers.
    "relativeruler": false,  	// make line numbers display relatively. 

    "scrollbar": true,  		// display a scroll bar.  
    "scrollbarchar": "\u2590",  // character used for the scroll bar, requires Nerd Font. Fallback: "|"
    "scrollmargin": 3.0,  		// margin of the scroll bar
    "scrollspeed": 2.0,  		// amount of lines to scroll for one scroll event.

    "diffgutter": true,  // show per-line diff (e.g. git-modified) status
```

- Some terminal fonts don't support nerd characters. In that case the `statusLine` may render with garbled glyphs. It's recommended to swap out `status-separator` and `scrollbarchar` yourself — for example, change them to `│`.
- A Nerd Font or other powerline-compatible font is recommended.

### StatusLine
```json
    "statusformatl": "$[special] $(brand) $[dim]$sep $(filename) $(modified)$[normal]$sep$(overwrite) $(position) $(status.paste) | ft:$(opt:filetype) ",  
    "statusformatr": "$(bind:ToggleKeyMenu):keys, $(bind:ToggleHelp):help",  
```

### Clipboard
```json
    // ── Clipboard and Input ──
    "clipboard": "external",  // external: accesses clipboard via an external tool, such as xclip/xsel or wl-clipboard on Linux, pbcopy/pbpaste on MacOS, and system calls on Windows.
    "useprimary": true,  // micro will use the primary clipboard to copy selections in the background. This does not affect the normal clipboard using `Ctrl-c` and `Ctrl-v`.
    "paste": false,  // treat characters sent from the terminal in a single chunk as a paste event rather than a series of manual key presses.
```

If you're using microNeo on a Linux server over SSH, it's recommended to set `{"clipboard": "external"}`.

### Terminal
```json
    // ── Terminal and Colors ──
    "fakecursor": false,  // forces micro to render the cursor rather than the actual terminal cursor.
    "xterm": false,  // micro will assume that the terminal is `xterm-256color` regardless of what the `$TERM` variable actually contains.
    "truecolor": "auto",  // controls whether micro will use true colors (24-bit colors) 
```

`truecolor` looks much nicer.

---

## Markdown
```json
    // ── Markdown ──
    "mdtablealign": true,  // align table | columns (left/center/right).
    "mdtableborder": true,  // draw table border lines.
    "mdbolditalic": true,  // render **bold** and *italic* inline styles.
    "mdcodeblock": true,  // render ``` fenced code blocks with background and border.
    "mdheading": true,  // render # headings with bold and color.
    "mdlist": true,  // render - / * / 1. list markers.
    "mdlink": true,  // render [text](url) links with a distinct color.
```

---

## Options you rarely touch

### Main UI
```json
    "colorcolumn": 0.0,  		// if this is not set to 0, it will display a column at the specified column. 
    							//This is useful if you want column 80 to be highlighted special for example.
    "cursorline": true,  		// highlight the line that the cursor is on in a different color 
    "infobar": true,  	// enables the line at the bottom of the editor where messages are printed. 
    "keymenu": false,  	// display the nano-style key menu at the bottom of the screen. 
    "divchars": "|-",  	// characters used for the dividing line between vertical/horizontal splits.
    "divreverse": true, // the color for the characters displayed in split dividers.
    "tabreverse": true, // reverses the tab bar colors when active.
    "tabhighlight": false,  // inverts the tab characters colors with respect to the tab bar.


    "basename": true,  // in the infobar and tabbar, show only the basename of the file being edited 
    "syntax": true,  // enables syntax highlighting.
```

### Editing
```json
    // ── Edit ──
    "autoindent": true,  		// when creating a new line, use the same indentation as the previous line.
    "keepautoindent": false,  	// when using autoindent, whitespace is added for you. 
    "smartpaste": true,  		// add leading whitespace when pasting multiple lines. 
    "softwrap": true,  			// wrap lines that are too long to fit on the screen. 
    "wordwrap": false,  		// wrap long lines by words, i.e. break at spaces. 
    "showchars": "",  			// sets what characters to be shown to display various invisible characters in the file.
    "tabsize": 4.0,  			// the size in spaces that a tab character should be displayed with.
    "tabstospaces": false,  	// use spaces instead of tabs. 
    "tabmovement": false,  		// navigate spaces at the beginning of lines as if they are tabs 
    "indentchar": " ",  		// sets the character to be shown to display tab characters. 

    // ── Search ──
    "incsearch": true,  // enable incremental search in "Find" prompt (matching as you type).
    "hlsearch": false,  // highlight all instances of the searched text after a successful search.
    "ignorecase": true,  // perform case-insensitive searches.

    // ── Brace ──
    "matchbrace": true,  	// show matching braces for '()', '{}', '[]' when the cursor is on a brace character 
    "matchbraceleft": true, // simulate I-beam cursor behavior
    "matchbracestyle": "highlight",  // whether to underline or highlight matching braces when `matchbrace` is enabled.

    "hltaberrors": false,  	// highlight tabs when spaces are expected, and spaces when tabs are expected.
    "hltrailingws": false,  // highlight trailing whitespaces at ends of lines. 
```

### File operations
```json
    // ── Files ──
    "autosave": 0.0,  // automatically save the buffer every n seconds, where n is the value of the autosave option.
    "autosu": false,  // When file saved but user doesn't have permission, micro will ask if use super user
    "sucmd": "sudo",  // specifies the super user command. 
    "backup": true,   // automatically keep backups of all open buffers. 
    "backupdir": "",  // the directory backups in. Value of `""` = `ConfigDir/backups`
    "permbackup": false,  // this option causes backups (see `backup` option) to be permanently saved.
    "eofnewline": true,  // micro will automatically add a newline to the end of the file if one does not exist.
    "rmtrailingws": false,  // micro will automatically trim trailing whitespaces at ends of lines.
    "mkparents": false,  // if a file is opened on a path that does not exist,
    "fileformat": "unix",  // this determines what kind of line endings micro will use for the file.
    "encoding": "utf-8",  // the encoding to open and save files with. 
    "fastdirty": false,  // this determines what kind of algorithm micro uses to determine if a buffer is modified.
    "readonly": false,  // when enabled, disallows edits to the buffer.
    "savecursor": false,  // remember where the cursor was last time the file was opened
    "saveundo": false,  // when this option is on, undo is saved even after you close a file
    "savehistory": true,  // remember command history between closing and re-opening micro.
    "reload": "auto",  // The available options are `prompt`, `auto` & `disabled`.  
```

### Multi-Panes
```json
    // ── Buffer and window ──
    "splitbottom": true,  // when a horizontal split is created, create it below the current split.
    "splitright": true,  // when a vertical split is created, create it to the right of the current split.
    "helpsplit": "hsplit",  // sets the split type by the `help` command. Possible values: * `vsplit`:  `hsplit`
    "multiopen": "tab",  // specifies how to layout multiple files opened at startup. 
    "parsecursor": false,  // parse filenames `file.txt:10:5` as open `file.txt` with the cursor at line 10 and column 5.
    "pageoverlap": 2.0,  // the number of lines from the current view to keep in view when paging up or down.
```

### Others
```json
    // ── Plugin ──
    "pluginchannels": ["https://raw.githubusercontent.com/micro-editor/plugin-channel/master/channel.json"],  // list of URLs pointing to plugin channels for downloading and installing plugins.
    "pluginrepos": [],  // a list of links to plugin repositories.

    // ── Others ──
    "detectlimit": 100.0,  // if this is not set to 0, it will limit the amount of first lines in a file that are matched to determine the filetype.
    "filetype": "unknown",  // sets the filetype for the current buffer. Set this option to `off` to completely disable filetype detection.
    "lockbindings": false,  // prevent plugins and lua scripts from binding any keys. Any custom actions must be binded manually either via commands like `bind` or by modifying the `bindings.json` file.
```
