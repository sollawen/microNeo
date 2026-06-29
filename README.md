
# <img src="./docs/website/assets/microNeo-logo-mark.svg" style="width:48px;height:auto" alt="microNeo logo" align="absmiddle"/> microNeo -- AI Partner

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](./LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.19+-00ADD8.svg)](https://golang.org/)
[![Single Binary](https://img.shields.io/badge/single%20binary-yes-green.svg)]()
[![awesome-tuis](https://awesome.re/mentioned-badge.svg)](https://github.com/rothgar/awesome-tuis)
[![中文说明](https://img.shields.io/badge/📖-中文说明-red)](https://sollawen.github.io/microNeo/)

## The terminal editor that can discuss with AI agents

Since vibe coding, I write code by hand less and less, and spend more and more time discussing with the AI. I always need to tell the AI exactly which part of a document I have thoughts about. I have to ctrl-c/ctrl-v all day long — it's given my fingers tendonitis.

So here comes **microNeo, an AI Partner**.

- Open a markdown document with microNeo and select the text you want to comment on
- Press `alt-enter` to open the input box, and write down your thoughts
- Press `alt-enter` again to send it to the AI. The AI will then receive your comment.

[![demo](./docs/website/assets/aibp-opencode.gif)](https://sollawen.github.io/microNeo/)

Currently supports `pi` and `opencode`; support for `claude cli` is under development.

---

## One-line Install

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/tools/install.sh | sh
```

- Fully supported on Linux/Mac. Windows requires a terminal command-line environment; not tested yet.
- See [Quick Start](https://sollawen.github.io/microNeo/en/quick-start/) for how to use microNeo.

---

## Features

- Full-featured terminal editor with syntax highlighting for 100+ languages 
- Communicate with AI agents to send your thoughts to the AI. Supports multiple AI agents.
- Markdown real-time rendering in the same window — comfortable for reading AI-written plan documents.
- Mouse support. Shortcuts are similar to VS Code.
- Small and fast — only 13 MB.


---


**Relationship with Micro**

microNeo originated from [Micro](https://github.com/micro-editor/micro). The codebase inherits Micro's editor architecture (zero dependencies, intuitive operation, Lua plugins, mouse support) and adds many features for vibe coding on top of it.

microNeo is now developed independently, with the goal of becoming the best AI agent partner in the terminal.


**License** -- [MIT](./LICENSE)

---

Email: sollawen@gmail.com
