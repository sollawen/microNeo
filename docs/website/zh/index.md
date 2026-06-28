# <img src="/microNeo/assets/microNeo-logo-mark.svg" style="width:48px;height:auto" alt="microNeo logo" align="absmiddle"/> microNeo -- AI 伴侣

## 可以和AI一起讨论方案的终端编辑器

AI编程之后，我几乎不再手敲代码了，而是把大部份时间花在了与AI讨论方案、制定计划上面。我经常需要告诉AI，我对方案里的具体哪个内容有想法。而这是传统编辑器很难做到的。

于是，就有了**microNeo AI伴侣**。

- 用microNeo打开一个markdown文档，select你想发表意见的那部份文字
- 按`alt-enter`打开输入框，在里面写下你的意见
- 写好之后，再次按`alt-enter`发送给AI。

AI就知道你是对哪段文字发表了意见了。

<video src="/microNeo/assets/aibp-opencode_h265_x265_crf28.mp4" controls autoplay muted loop playsinline style="width:100%;max-width:880px;border-radius:8px"></video>

目前支持`pi`和`opencode`，对`claude cli`的支持正在开发中。

---

## 安装

```bash
curl -fsSL https://raw.githubusercontent.com/sollawen/microNeo/master/install.sh | sh
```

- 完美支持Linux/Mac。Windows需要终端命令行的运行环境，没有实测
- 请查阅 [QuickStart](quick-start.md) 开始使用 MicroNeo

---

## microNeo 亮点
- 完整功能的 terminal editor，支持100+语法高亮，支持鼠标。快捷键与VScode相似
- 支持markdown同屏实时渲染，阅读AI写的方案文档，非常舒服
- 能与AI agent通信，把你的意见发送给AI。支持同时与多个AI通信
- 非常小巧快速，只有 13 MB


---


**与 Micro 的关系**

microNeo 起源于 [Micro](https://github.com/micro-editor/micro) ，代码继承了 Micro 的编辑器架构（零依赖、操作直观、Lua 插件、鼠标支持），并在上面新增了很多AI时代的功能。

microNeo 现在独立开发，目标是成为终端里最好的 AI agent 的伴侣(partner)。


**许可证** -- [MIT](https://github.com/sollawen/microNeo/blob/master/LICENSE)

---

Email: sollawen@gmail.com






