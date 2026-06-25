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
- 包含"aibp-opendoe"的就是源代码

你觉得这个逻辑怎么样？
