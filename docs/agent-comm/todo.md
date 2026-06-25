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

@aibp-agents/opencode/README.md 


---

# 问题的现象
- aibp-opencode 如果用源代码目录的方式安装到opencode里成为插件，就可以正确运行，注册出名字并显示出来
- 但如果用npm:aibp-opencode的方式安装最新版1.0.2到opencode里面的话，opencode无法识别出来安装了这个插件，所以启动后根本就没有理会这个插件，也就是说没有注册出名字，更没有显示
- 现在这个目录里的代码，与npm:aibp-opencode v1.0.2是一模一样的

# 我的猜测
我认为在opencode里面安装npm:aibp-opencode的方式肯定是有问题，让opencode无法识别
- 我们的这个插件，是插在tui.json里的，没有插在opencode.json里面
- 但是奇怪的是用源代码目录的方式安装就可以正确识别出来

# 你来debug一下

- 你可以用bash来install/uninstall to opencode，试验各种不同的安装方法，看看哪个有效
- 你可以使用gh到https://github.com/anomalyco/opencode 里去查找有关的代码和文档
- 你可以使用curl查看 https://opencode.ai/docs/zh-cn/plugins/ 的插件说明
- 你可以查看git历史，最早的npm:aibp-opencode v1.0.0是可以正常安装和工作的
- 不要修改microNeo目录里的代码文件和文档

# 基本原则

- opencode是用户非常多的开源软件。它的npm安装插件的机制是非常成熟的。成千上万的人在写npm的opencode插件。
- 所以，opencode的npm插件一定是能够非常简单的安装和使用的
- 我们的aibp有问题，一定是我们的代码或是package.json没有符合opencode的插件规范
