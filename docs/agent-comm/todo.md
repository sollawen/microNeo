# 20260625
任务1
- [x] 了解并测试 pi 如何通过命令行的命令，update npm package
- [x] 了解并测试 opencode 如何 update npm package

任务2
- [x] 把pi改成安装源代码路径的aibp
- [x] 把opencode改成安装源代码路径的aibp

任务3
- [x] 修改aibp-pi 里面的目录名称，aibp协议改成2.0
- [x] 修改aibp-opencode 里面的目录名称，aibp协议改成2.0
- [x] 修改microNeo端的协议注册目录名字，aibp协议版本改成2.0

任务4
@docs/agent-comm/工作记录0624.md 
@docs/agent-comm/工作记录0625.md 

这是昨天和今天的完成的工作。

## 我们下面的任务

每个 ensure_{agent}.go 要提供的接口函数
- getAIBPversion()
    - 读取本agent已经安装的aibp的版本号
    - 返回值 (大号，小号，isSourceInstall)
    - 如果无法识别本agent安装的aibp，(0,0, false)
- installAIBP()
    - 安装最新版本的npm包到本agent里面
- updateAIBP()
    - 升级npm包到最新的版本
    - 注意opencode是需要清除老版本catch的，好像pi不需要

修改 ensure.go 里面对 :check-agent 命令的执行逻辑
```
    for each agent {
        version = agent.getAIPBversion()
        if version isSourceInstall then next

        if version 无法识别 {
            agent.installAIBP
            next
        }
        if version.大号 < microNeo.大号 {
            agent.updateAIBP
            next
        }
        if version.大号 > microNeo.大号 {
			infoBar 提示用户
			next
		}
        
    }
```

你做为CTO，写个实施计划吧


```go
   aibp-pi.AIBPVersion() {
       先读setting.json
       if 里面的aibp扩展项 == 源代码路径 {
           return 0,0,true
       }
       if 里面的aibp扩展项 == npm包 {
           大号，小号 = getVersionFromNpm()
           return 大号，小号，false
       }
       return 0,0,false
   }
```


对pi来说
- 包含"aibp-agents"的，就是源代码路径
- 包含"npm:aibp-pi"的，就是npm包

对opencode来说
- 包含"aibp-agents"的就是源代码路径
- 包含"aibp-opendoe"的就是npm包

### 意外发现并修复：D6 bug（计划外）

 测试跑起来时发现 npm_package / npm_pinned_version 返回 isSource=true，根因是老代码：

 ```go
   // opencodeNpmAIBPVersion 原来的写法
   return aibp.ParseProtocol(pkg.AIBP.Protocol)  // ❌ ParseProtocol 第三返回值是 ok，被当成 isSource
 ```

 ParseProtocol 返回 (major, minor int, ok bool)，ok 是「解析成功」，不是 isSource。npm 安装的 aibp 解析成功 → ok=true → Ensure
 误判为 source → 跳过所有更新检查。这是预先存在的真实 bug（改 D3 时才暴露，因为之前硬读路径的行为凑巧）。

 修复：显式拆开返回值，npm 安装恒 isSource=false。这是 opencode 测试能全绿的关键。

 ### 验证结果

 - ✅ go vet ./internal/aibp/... 通过
 - ✅ opencode 9 个测试全过（TestOpencodeAIBPVersion 全部子用例）
 - ✅ make build-quick 编译通过
 - ⚠ pi 测试失败（TestPiAIBPVersion/npm_package + npm_pinned_version）—— 已用 git stash 确认在 HEAD 上就失败，是 pi 的同一个 D6
   bug（piNpmAIBPVersion 第 104 行同样误用 ParseProtocol）。按你的指示没碰 pi。

 ### 关于 pi 的 D6 bug

 ensure_pi.go:104 有完全一样的 return aibp.ParseProtocol(...)。修复方式和我对 opencode 做的一样（3 行）。你想修的话我随时能做，
 但需要你确认——因为它会让 pi 的 npm 安装也能被正确检测更新。这是个独立的小修，不影响本次 opencode 的交付。
