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
**修改MD文件的 Relocate()**, 
```
func Relocate() {
	if cursor所在的Line发生了+1 and 新line与旧line不是同一个segment {
		做一次preRender，重新计算viewportRowmap
	}
	根据vewportRowmap and cursor所在的新line，重新计算startLine
}
```

**修改所有的render**, 
- 都提供一个driRun的方式 渲染出来的是几个row，这些row和line的对应关系
- preRender时使用dryRun

