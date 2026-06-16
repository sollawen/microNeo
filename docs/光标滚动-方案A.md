@docs/光标滚动-修改总结.md 

# 一、现在有个遗留的问题：

## 问题的现象
以docs/sample.md为例
- line55-59是个table，
	- 在原生编辑状态时只需要显示5 row
	- 但是在render模式时会显示9 row, 因为需要好几个表格frame的装饰row
	- 这是正确的
- 假设光标在line59,且line62是viewport显示的最后一行
	- 因为光标在line59，所以这个table一定是处在原生的编辑状态下，没有装饰行
	- 这时，我按下键，于是程序判断需要scroll up 1 row了
	- 于是整个buffer scrolled up 1 row, 光标移到了line60

这个时候，问题出现了
- 因为光标移到了line60，脱离了这个table了。所以整个table恢复到了渲染模式
- 而渲染模式会添加好几个装饰行，这导致了这个table的显示，一下子就变大了，下面的部份就跑到viewport的底部之外了
- 光标就更是在viewport底部的好几个row之外了，viewport里面看不见了。
- 然后再继续按下键的时候，光标就会在viewport的底边之外向下移动了。用户是看不见的。
- 除非用户按上键，把光标移回到viewport里面，才能看到

# 二、我的观点
- 我们的render把table添加装饰行进行渲染，是正确的，工作的很漂亮
- 光标在line59的时候，按下键后判断应该scroll up，也是非常正确的
- 但两个正确，得到的结果却是不正确的。

## 根本原因
在scrollup的时候，或者说在scrollup之前
- line55-59这个table是处在原生编辑状态下的，没有装饰行
- 所以scrollup 1 row的时候，是根据没有装饰行的viewportRowmap来计算应该向上滚动多少row

但是scrollup完成后
- 这个table变成了渲染状态，凭空多出来了几个装饰行
- 导致按原有数据计算后scrollup 的row数量不够，光标就跑出底部了

## 方案A，preRender
**修改MD文件的 Relocate()**, 当cursor所在的Line发生了 +1 的变化的时候，
- 做一次preRender，重新计算viewport的row->line的对应关系, 存放在某个变量里
- 然后根据 这个变量 and cursor所在的line，重新计算startLine

**修改所有的render**, 都提供一个接口函数 preRender() return 渲染出来的是几个row，这些row和line的对应关系

# 三，micro原生的时序

因为micro原生代码，是一个line editor，所以除了softwrap之外，根本不存在render的事情，line/row是精确的一一对应的关系。因为，micro原生代码做了一个非常简洁的处理
```
	while true {
	    display(startLine)            // 每帧都从头重算：从 startLine 开始逐行扫 buffer，算 tab/wrap/高亮，填入屏幕
	    screen.Show()
	    event = <-screen.Events
	    handleEvent(event) {
	        case A: 状态变更（编辑 / 移动光标 / 跳转）
	            → 改 buffer 或光标
	            → 调 Relocate()
	            → Relocate 判断光标是否出安全区：
	                出界 → 改 startLine
	                在安全区 → startLine 不变（这是常态）
	        case B: 纯视口（scroll / page / center / start / end）
	            → 直接设 startLine（必然变）
	            → 不碰 buffer / 光标，不调 Relocate
	    }
	}
```

**关键观察：原生 display 是一个纯函数 `f(startLine, buffer) → 屏幕内容`，每帧全量重算，没有任何缓存。** 所以用户连按三次右键，就会从头重算三次——这对 line editor 来说既便宜又正确（buffer 一改，下一帧自动反映，完全不用操心同步）。

## markdown 文件的特别之处
markdown文件渲染后，line/row之间不再是精确的一一对应关系了，某些segment会产生不同数量的装饰行。所以md文件需要有好几个不同的render来计算 buffer.line变成显示后的数据是什么样子

因此，方案B需要引入一个 preRenderedMap，记录viewport从第一个row开始，到光标所在line为止，每个row对应的line的关系
- 从startLine开始，调用preRender, 生成 preRenderedMap


## 四、方案A的伪代码
