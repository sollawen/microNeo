
microNeo 常用的命令行参数，用来在不打开编辑器的情况下做配置维护和 AI agent 相关的检查、升级。


### `reset-settings` — 重置配置

```bash
microneo --reset-settings
```

在用户配置目录里把 `settings.json` 恢复成 microNeo 自带的默认值。
- 现有的 `settings.json` 会先备份成 `settings.json.backup`，再写入新文件
- 适合配置改乱了、或者升级之后字段对不上的时候用


### `check-agent` — 检查并修复 AI agent 扩展

```bash
microneo --check-agent
```

microNeo 通过在 AI agent（pi、opencode、claude cli）安装一个极小的 aibp 通信协议扩展来实现与microNeo之间的通信。

这个命令会：
- 扫一遍电脑上都装了哪些 AI agent
- 对每个 agent 检查 aibp 扩展是否已经装好、版本是不是最新
- 缺了就装，旧了就升
- 干完直接退出，不会进编辑器

**什么时候跑一下：**

- 第一次装完 microNeo 之后
- 新装了一个 AI agent 之后
- 跟 agent 连不上、或者发消息没反应的时候

---

### `update-aibp` — 只升级 AI agent 扩展

```bash
microneo --update-aibp
```

只做一件事：把 aibp 扩展升到最新版本，不做其它检查。

- 像 pi 这种 agent 自己会升级 aibp，不用管它
- opencode、claude cli 不会自更新 aibp，就需要手动跑一下这个命令
- 升级完直接退出

以后我考虑把这个命令与上一个命令合并。
