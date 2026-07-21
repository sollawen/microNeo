
microNeo 的所有键位都写在一个 JSON 文件里，用户可以自由修改。

### 文件位置

```
~/.config/microNeo/bindings.json
```

启动时用 `--config <dir>` 可以临时改配置目录。

### 文件结构

bindings.json 顶层是一个对象，每个 key 对应一种 pane 类型（编辑器区域）。microNeo 启动时按以下顺序读：

1. 每个 section 的**默认值**（microNeo 内置）
2. 用户在 bindings.json 里的**自定义**配置

**后写的覆盖前写的**。所以默认 `Ctrl-S` 是 `Save`，用户在文件里写 `"Ctrl-S": "Quit"` 后，`Ctrl-S` 就变成 `Quit` 了。

### 四个 section

| Section | 作用 |
|---|---|
| `buffer` | 主编辑器（编辑文件、写代码） |
| `command` | 命令行模式（按 Ctrl-E 进入，按 : 输入命令） |
| `notepane` | notePane（与 AI 对话的输入框） |
| `terminal` | 终端面板 |

### 顶层写法 vs section 写法

两种写法都可以：

```json
{
    "Ctrl-S": "Save"
}
```

等价于：

```json
{
    "buffer": {
        "Ctrl-S": "Save"
    }
}
```

顶层写法默认走 buffer section。**要配置 notePane 或 command 的专属 action，必须用 section 写法**，否则会报错（顶层会被当作 buffer section）。

### 常用示例

#### 1. 换 notePane 的发送键

默认 `Alt-Enter` 是发送键，想换成 `F5`：

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

- **buffer section**：`F5` 在主编辑器里打开 notePane
- **notepane section**：`F5` 在 notePane 里发送草稿

两个 section **各自独立**，要换键必须写两行。

#### 2. 禁用某个键

把 `Alt-Enter` 禁掉（变成 no-op）：

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

- `None` 是 microNeo 的特殊 action —— "不响应"。这是"换键"的标准写法：先禁用旧键，再绑新键。
- 和vim等其它编辑器一样，microNeo支持**多个 key 对应同一个 action**（比如 `Ctrl-H`、`Backspace`、`Shift-Backspace` 都映射到 `Backspace`）。所以如果用户希望自己的新定义的键覆盖掉系统预设的键，需要明确的写上预设的键为 "None"

#### 3. 改通用编辑键

notePane 自动跟随主编辑器的通用编辑键配置：

```json
{
    "buffer": {
        "Enter":   "None",
        "Alt-a":   "StartOfLine",
        "Tab":     "InsertTab"
    }
}
```

- notePane 里 `Enter` 被禁用
- `Alt-a` 在 notePane 里也跳到行首（默认是 `StartOfText`）
- `Tab` 走复合链

#### 4. notePane 的 Esc 不能改

`Esc` 在 notePane 里硬绑为 `NotePaneClose`，**不会**被用户的 `Esc: None` 覆盖。这是 microNeo 的保护机制，防止用户把自己关在 notePane 里出不来。


### 完整 action 列表

想查所有可用的 action，可以运行：

```
:command help
```

或参考 micro 原生文档：[micro Bindings](https://github.com/zyedidia/micro/blob/master/runtime/help/bindings.md)。

### 常见问题

#### 我的 bindings.json 报错怎么办？

检查 JSON 格式：
- 末尾有没有 `,`
- 引号是不是英文双引号
- 键名是不是 `"Alt-Enter"` 这种格式（不是 `<Alt-Enter>`）


#### 改了 bindings.json 没生效？

1. 看文件路径对不对（`~/.config/microNeo/bindings.json`，不是 micro 原生位置）
2. 看 JSON 格式对不对
3. 重启 microNeo 试试
