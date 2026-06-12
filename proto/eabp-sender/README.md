# EABP Sender Prototype (一次性原型)

>⚠️ **一次性原型** — 验证完即退役，不进 microNeo 主线。

## 用途

模拟 EABP 发送端，用于验证 EABP 协议端到端通信：

- `cmd/discover` — 扫描注册表，发现存活的 receiver
- `cmd/send` — 向指定 receiver 发送上下文报文

## 目录结构

```
go/
├── go.mod
├── registry.go        # 注册表发现逻辑（验证后搬入 internal/eabp/）
├── message.go        # EABP 信封/载荷结构（验证后搬入 internal/eabp/）
└── cmd/
    ├── discover/     # 发现工具
    └── send/        # 发送工具
```

## 编译与使用

```bash
cd proto/eabp-sender/go
go build ./cmd/discover ./cmd/send

# 发现所有存活的 receiver
./discover

# 发送带消息的上下文
./send -name pi-<pid> -msg "这段标题怪，换个说法" -file a.md -line 42

# 发送纯上下文（不触发对话）
./send -name pi-<pid> -file a.md -line 42
```

## 相关文档

- 协议详情：`docs/agent-comm/D2-通信协议方案.md`
- 原型验证计划：`docs/agent-comm/D3-原型验证计划.md`