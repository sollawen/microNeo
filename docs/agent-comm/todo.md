# 20260625
任务1
- [x] 了解并测试 pi 如何通过命令行的命令，update npm package
- [x] 了解并测试 opencode 如何 update npm package

任务2
- [x] 把pi改成安装源代码路径的aibp
- [x] 把opencode改成安装源代码路径的aibp

任务3
- [x] 修改aibp-pi 里面的目录名称，aibp协议改成1.1
- [x] 修改aibp-opencode 里面的目录名称，aibp协议改成1.1
- [x] 修改microNeo端的协议注册目录名字，aibp协议版本改成1.1

任务4
- [ ] 协议比较改 x.x
  - [ ] 新增 ParseProtocol(s) (major, minor int, ok bool)，替换 MajorVersion
  - [ ] ensure.go 比较逻辑改用 ParseProtocol，x.x 任一不同 → 需要升级
  - [ ] registry.go 的 discover 协议匹配同步改
  - [ ] 测试断言更新到 aibp-1.1
- [ ] AgentEnsurer 接口加 UpdateAIBP() error
- [ ] PiEnsurer.UpdateAIBP() 实现（命令待 D 方案确认）
- [ ] OpencodeEnsurer.UpdateAIBP() 实现（rm cache + plugin add -g）
- [ ] Ensure() 编排：protocol 一致后调用 UpdateAIBP（idempotent，无脑调）
- [ ] npm publish aibp-pi@1.1.0 和 aibp-opencode@1.1.0

