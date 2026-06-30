# MicroNeo default settings (JSON5)

## 目录
microNeo 的用户配置文件：`~/.config/microNeo/settings.json`

重置为缺省的、完整的、全套 settings.json:
```bash
# bash
# 这个命令会生成一套完整的 ~/.config/microNeo/settings.json

microneo --reset-settings
```

## 比较常用的配置项

### Basic
```json
    // ── theme ──
    "colorscheme": "s-dark",		// 我自己写的 Dark Mode  
    "mouse": true,  				// mouse support.
```

`s-dark` and `s-light` 是我自己写的theme，一个亮色系，一个暗色系

### 编辑主界面 Main UI
```json
	// UI settings    
    "statusline": true,  			// display the status line at the bottom of the screen.
    "status-separator": "\ue0b0",  	// string substituted into the $sep placeholder inside statusLine

    "ruler": true,  			// display line numbers.
    "relativeruler": false,  	// make line numbers display relatively. 

    "scrollbar": true,  		// display a scroll bar.  
    "scrollbarchar": "\u2590",  // specifies the character used for displaying the scrollbar.  
    "scrollmargin": 3.0,  		// margin of the scroll bar
    "scrollspeed": 2.0,  		// amount of lines to scroll for one scroll event.

    "diffgutter": true,  // 显示diff，该行是否被修改过
```

- 有的终端字体不支持 `nerd` 字符。这时`statusLine`显示会有一点乱码。建议自行更换 `status-separator` and `scrollbarchar`，例如改成 "│"
- 推荐使用 Nerd Font 或其他兼容 powerline 的字体

### StatusLine
```json
    "statusformatl": "$[special] $(brand) $[dim]$sep $(filename) $(modified)$[normal]$sep$(overwrite) $(position) $(status.paste) | ft:$(opt:filetype) ",  
    "statusformatr": "$(bind:ToggleKeyMenu):keys, $(bind:ToggleHelp):help",  
```

### 剪贴板
```json
    // ── Clipboard and Input ──
    "clipboard": "external",  // external: accesses clipboard via an external tool, such as xclip/xsel or wl-clipboard on Linux, pbcopy/pbpaste on MacOS, and system calls on Windows.
    "useprimary": true,  // micro will use the primary clipboard to copy selections in the background. This does not affect the normal clipboard using `Ctrl-c` and `Ctrl-v`.
    "paste": false,  // treat characters sent from the terminal in a single chunk as a paste event rather than a series of manual key presses.
```

如果你通过ssh登录到Linux服务器上使用 mincroNeo，建议设置成 `{"clipboard": "external"}`

### 终端设置
```json
    // ── Terminal and Colors ──
    "fakecursor": false,  // forces micro to render the cursor rather than the actual terminal cursor.
    "xterm": false,  // micro will assume that the terminal is `xterm-256color` regardless of what the `$TERM` variable actually contains.
    "truecolor": "auto",  // controls whether micro will use true colors (24-bit colors) 
```

`truecolor` 要好看很多

---

## Markdown 相关
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

## 不常修改的配置项

### 编辑主界面 Main UI
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

### 编辑功能
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

### 文件操作
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

### 多窗口 Multi-Panes
```json
    // ── Buffer and window ──
    "splitbottom": true,  // when a horizontal split is created, create it below the current split.
    "splitright": true,  // when a vertical split is created, create it to the right of the current split.
    "helpsplit": "hsplit",  // sets the split type by the `help` command. Possible values: * `vsplit`:  `hsplit`
    "multiopen": "tab",  // specifies how to layout multiple files opened at startup. 
    "parsecursor": false,  // parse filenames `file.txt:10:5` as open `file.txt` with the cursor at line 10 and column 5.
    "pageoverlap": 2.0,  // the number of lines from the current view to keep in view when paging up or down.
```

### 其它
```json
    // ── Plugin ──
    "pluginchannels": ["https://raw.githubusercontent.com/micro-editor/plugin-channel/master/channel.json"],  // list of URLs pointing to plugin channels for downloading and installing plugins.
    "pluginrepos": [],  // a list of links to plugin repositories.

    // ── Others ──
    "detectlimit": 100.0,  // if this is not set to 0, it will limit the amount of first lines in a file that are matched to determine the filetype.
    "filetype": "unknown",  // sets the filetype for the current buffer. Set this option to `off` to completely disable filetype detection.
    "lockbindings": false,  // prevent plugins and lua scripts from binding any keys. Any custom actions must be binded manually either via commands like `bind` or by modifying the `bindings.json` file.
```

