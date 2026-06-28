# <img src="/microNeo/assets/microNeo-logo-mark.svg" style="width:48px;height:auto" alt="microNeo logo" align="absmiddle"/> microNeo -- AI Partner

[![MIT License](https://img.shields.io/badge/license-MIT-blue.svg)](https://github.com/sollawen/microNeo/blob/master/LICENSE)
[![Go Version](https://img.shields.io/badge/Go-1.19+-00ADD8.svg)](https://golang.org/)
[![Single Binary](https://img.shields.io/badge/single%20binary-yes-green.svg)]()
[![awesome-tuis](https://awesome.re/mentioned-badge.svg)](https://github.com/rothgar/awesome-tuis)
[![中文](https://img.shields.io/badge/📖-中文-red)](https://sollawen.github.io/microNeo/)

## The terminal editor for discussing plans with AI

In the era of AI coding, I almost never write code by hand anymore. Instead, I spend most of my time discussing plans and strategies with the AI. I often need to tell the AI exactly which part of a plan I have thoughts about — something traditional editors can hardly do.

So here comes **microNeo AI Partner**.

- Open a markdown document with microNeo and select the text you want to comment on
- Press `alt-enter` to open the input box, and write down your thoughts
- Press `alt-enter` again to send it to the AI. The AI will then receive your comment.

<video src="/microNeo/assets/aibp-opencode_h265_x265_crf28.mp4" controls autoplay muted loop playsinline style="width:100%;max-width:880px;border-radius:8px"></video>

Currently supports `pi` and `opencode`; support for `claude cli` is under development.

---

## One-line Install

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/install.sh | sh
```

- Fully supported on Linux/Mac. Windows requires a terminal command-line environment; not tested yet.
- See [Quick Start](quick-start.md) for how to use microNeo.

---

## microNeo Highlights
- Full-featured terminal editor with syntax highlighting for 100+ languages and mouse support. Shortcuts are similar to VS Code.
- Markdown real-time rendering in the same window — comfortable for reading AI-written plan documents.
- Communicate with AI agents to send your thoughts to the AI. Supports talking to multiple AIs at once.
- Small and fast — only 13 MB.


---


**Relationship with Micro**

microNeo originated from [Micro](https://github.com/micro-editor/micro). The codebase inherits Micro's editor architecture (zero dependencies, intuitive operation, Lua plugins, mouse support) and adds many features for the AI era on top of it.

microNeo is now developed independently, with the goal of becoming the best AI agent partner in the terminal.


**License** -- [MIT](https://github.com/sollawen/microNeo/blob/master/LICENSE)

---

Email: sollawen@gmail.com
