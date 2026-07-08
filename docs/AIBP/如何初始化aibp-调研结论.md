# 如何初始化 aibp —— 调研结论

> 本文是对 [`如何初始化aibp.md`](./如何初始化aibp.md) 的回答。
>
> **状态**：调研完成，方案待用户拍板。
>
> **结论先行**：用户的想法大方向正确（文件归 aibp 自己管 + copy），但有一个关键误解——**光 copy 文件到任意目录 pi 发现不了**。必须额外做一步注册。推荐用 `pi install <本地路径>` 完成注册（方案 4）。

---

## 一、TL;DR

| 用户的问题 | 调研结论 |
|------------|----------|
| 如何发现用户装了 pi | `exec.LookPath("pi")` 在 PATH 找可执行文件；辅助检查 `~/.pi/` 是否存在 |
| 如何判断 pi 装没装 aibp | 读 `~/.pi/agent/settings.json` 的 `packages` 数组，看有没有指向 aibp-pi 的条目 |
| 如何把 aibp 装进 pi | **部署文件到 `$XDG_CONFIG_HOME/aibp/agents/pi/` + 调 `pi install <绝对路径>`**（方案 4） |

**推荐方案 4**（用户想法的修正版），长期可演进到方案 3（发布 npm/git）。

---

## 二、pi 扩展机制的关键事实（调研依据）

> 来源：`/opt/homebrew/lib/node_modules/@earendil-works/pi-coding-agent/docs/packages.md` + `extensions.md` + 本机实测。

### 2.1 pi 只从这些位置发现扩展

| 位置 | 说明 |
|------|------|
| `~/.pi/agent/extensions/*.ts` | 约定目录（全局，单文件） |
| `~/.pi/agent/extensions/*/index.ts` | 约定目录（全局，子目录） |
| `.pi/extensions/*.ts` / `*/index.ts` | 项目本地约定目录 |
| `settings.json` 的 `packages` 数组 | npm: / git: / 本地路径 |
| `settings.json` 的 `extensions` 数组 | 本地路径（文件或目录） |

**pi 不会扫描任意目录**。这是用户原想法的关键卡点。

### 2.2 `pi install` 命令（本机实测）

```
pi install <source> [-l] [--approve|--no-approve]
  source: npm:@foo/bar | git:github.com/u/r | ./local/path | /abs/path
  -l: 写到项目级 .pi/settings.json（默认写用户级 ~/.pi/agent/settings.json）
```

- **本地路径安装不复制文件**（文档原文 "added to settings without copying"）——只在 settings.json 登记一个路径引用，源文件必须在原地不动
- 命令本身非交互，microNeo 可以直接 `exec.Command("pi", "install", absPath)` 调用

### 2.3 本机现状（印证问题存在）

当前 `~/.pi/agent/settings.json` 里：
```json
"packages": [
  "../../pi-dev/pi-in-zellij",
  "../../pi-dev/pi-to-chrome",
  "../../pi-dev/microNeo/aibp-agents/pi"   // ← 开发者机器的相对路径
]
```
**普通用户机器上根本没有 `~/pi-dev/microNeo/...` 这个路径**。这正是"初始化 aibp"要解决的问题：用户装了 microNeo 后，怎么把这个登记项变成一个有效路径。

### 2.4 aibp-pi 当前的 package.json

```json
{
  "name": "aibp-pi",
  "private": true,
  "pi": { "extensions": ["./index.ts"] }
}
```

- ✅ 无 `dependencies`（运行时只 `import type`，不需要 `npm install`）→ 部署极轻量，copy 两个文件即可
- ❌ **缺 `version` 字段** → 版本检测做不了，需要补上

---

## 三、方案对比

| 方案 | 做法 | 文件归属 | `pi list` 可见 | 离线 | 升级路径 | 复杂度 |
|------|------|----------|---------------|------|----------|--------|
| **1. copy 到 pi 约定目录** | `~/.pi/agent/extensions/pi/index.ts` | pi 的地盘 | ❌ 无登记 | ✅ | microNeo 覆盖 | 低 |
| **2. copy 到 aibp 目录 + 手改 settings.json** | copy + 自己合并 json | aibp | ✅ | ✅ | 覆盖+合并 json | 中（json 合并易错） |
| **3. `pi install npm:` / `git:`** | 发布到 npm/git，调 `pi install` | pi 标准 | ✅ 最标准 | ❌ 需网络 | `pi update` | 低（但要先发布） |
| **4. copy 到 aibp 目录 + `pi install /local`** ⭐ | copy + `pi install <abs>` | aibp(文件)+pi(登记) | ✅ | ✅ | microNeo 覆盖文件 | 低 |

### 为什么推荐方案 4

1. **用 pi 的标准注册机制**：`pi install` 负责写 settings.json，microNeo 不用自己合并 json（避免破坏用户其它配置）
2. **文件所有权清晰**：aibp-pi 源码在 `$XDG_CONFIG_HOME/aibp/agents/pi/`，归 aibp 管；pi 只持有一个路径引用
3. **用户可见、可管理**：`pi list` 看得到，`pi remove` 能卸载，符合用户对"pi 扩展"的心智模型
4. **离线可用**：源码随 microNeo 分发，不需要网络
5. **升级简单**：microNeo 覆盖文件即可（pi 重启 / `/reload` 后生效）

### 为什么不选其它

- **方案 1**（约定目录）：settings.json 无记录，"这个扩展哪来的"不透明，用户 `pi remove` 找不到，卸载残留
- **方案 2**（手改 json）：要自己读-合并-写 settings.json，JSON 合并易破坏用户配置，重复造 `pi install` 的轮子
- **方案 3**（npm/git）：最干净，但需要发布 + 网络。**建议作为长期演进目标**，方案 4 作为离线兜底

---

## 四、推荐方案的实施流程（方案 4）

### 4.1 触发时机

microNeo 启动时（或用户首次触发"发送给 agent"时）执行一次检查。轻量、幂等。

### 4.2 三步流程

```
┌─────────────────────────────────────────────────────────┐
│ Step 1: 发现 pi                                          │
│   exec.LookPath("pi") → 找到？没找到 → 静默放弃（提示用户）│
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Step 2: 检测是否已装 aibp-pi                              │
│   读 ~/.pi/agent/settings.json 的 packages 数组          │
│   找有没有条目指向 aibp-pi（按 name 或路径匹配）          │
│   ┌─ 已装且版本新 → 结束                                  │
│   └─ 未装 / 版本旧 → 进入 Step 3                         │
└─────────────────────────────────────────────────────────┘
                          ↓
┌─────────────────────────────────────────────────────────┐
│ Step 3: 部署 + 注册                                       │
│   3a. copy 随包 aibp-agents/pi/ 的               │
│       index.ts + package.json                           │
│       → $XDG_CONFIG_HOME/aibp/agents/pi/               │
│       （fallback ~/.config/aibp/agents/pi/）           │
│   3b. exec: pi install <上面那个绝对路径>                 │
│       （首次安装才需要这步；升级只覆盖文件，不重装）       │
└─────────────────────────────────────────────────────────┘
```

### 4.3 目录布局（方案 4 落地后）

```
$XDG_CONFIG_HOME/aibp/
├── aibp-names.json          # 已有（D11 名字池）
└── agents/
    └── pi/                  # ⭐ 新增：aibp-pi 源码副本
        ├── index.ts
        ├── package.json     # 补 version 字段
        └── README.md

~/.pi/agent/settings.json    # pi 自动写入的登记项
└── packages: [ ".../aibp/agents/pi" ]
```

### 4.4 版本检测

- 给 `aibp-agents/pi/package.json` 补 `"version": "1.0.0"`
- microNeo 对比「随包 package.json 的 version」vs「已部署的 package.json 的 version」
- 过期 → 覆盖文件（pi 下次加载用新版，无需 `pi install` 重新注册——路径没变）

---

## 五、用户原想法的修正点

| 用户原想法 | 修正 |
|-----------|------|
| ❌ "copy 到 `xdg_config/aibp/pi/`，pi 就能用" | pi 不扫描任意目录。必须再 `pi install` 注册，或 copy 到 pi 约定目录 |
| ✅ "aibp 需要自己的目录" | 对。建议 `$XDG_CONFIG_HOME/aibp/agents/pi/`（和现有 `aibp-names.json` 同属 aibp 命名空间） |
| ✅ "目录不存在/版本低就 copy 覆盖" | 对。但需先补 package.json 的 `version` 字段才能比版本 |
| ➕ 用户没提到但需要的 | "如何发现 pi"（LookPath）+ "如何判断已装"（读 settings.json） |

---

## 六、待用户决策的开放问题

1. **触发时机**：microNeo 启动时自动检查，还是用户首次"发送给 agent"时才检查？
   - 启动时：体验好（用户无感），但每次启动有微小开销
   - 首次发送：延迟到需要时，但首次会发现"没装"打断流程
2. **未装 pi 时的行为**：静默放弃 + 日志？还是提示用户"建议安装 pi 以启用 agent 通信"？
3. **是否同时支持方案 3**（npm/git）：作为"有网络时优先用官方源、无网络时回退本地 copy"的双通道？还是 v1 只做方案 4？
4. **升级是否需要用户确认**：microNeo 自动覆盖 aibp-pi 文件，还是弹个确认？
5. **`pi install` 失败的兜底**：比如 settings.json 只读、pi 版本太旧不认 `pi install`——是否回退到方案 1（直接 copy 到约定目录）？

---

## 七、对 microNeo 代码侵入的评估

方案 4 的实现是**纯新增**，不动 micro 原生代码，符合项目"零侵入"原则：

- 新增 `internal/aibp/bootstrap.go`（或类似）：发现 pi + 检测已装 + 部署 + 注册的逻辑
- 在 microNeo 启动钩子（已有的 aibp 初始化路径）里调一次
- `aibp-agents/pi/package.json` 补 `version` 字段

预计代码量：~150 行 Go。无新依赖（`os`、`os/exec`、`encoding/json`、`path/filepath` 全是标准库）。
