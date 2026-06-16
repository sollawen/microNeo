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

preRender：可以不新增这个新函数，而是使用displayBufferMD()的dryRun模式
```
func preRender(光标的新位置) {
	从startLine开始
	render 每个line into viewportRowmap
		- 有的是用microNeo的render
		- 有的是用native的代码。原生代码里有一个计算一个line经过softwrap后是多少row的函数
	一直到退出条件达到为止
}
```

preRender里面退出循环的三个条件
1. 光标行 + scrollmargin 已渲染，
2. 旧光标段已整段渲染
3. viewport 已填满

停止条件 = 三个目的全部达成。


## “触发判定”的问题

场景

- 肯定是因为光标向下移动了，从oldLine移动到newLine, 且newLine=oldLine+1
- oldLine原先是处在原生编辑状态的（这是必然的，100%肯定，都不需要if判断）
- newLine是要进入原生编辑状态了，这也是必然的，不需要判断
- newLine和oldLine不属于同一个segment（渲染片）
    - 因为如果是同一个segment，那说明oldLine是需要仍然保持原生编辑状态的，不需要展开渲染
      ，viewportRowmap没有变化，也就根本不需要preRender了
    - 符合这个条件的，就是 table, code block, list
- 这里要注意的是：
    - 即使 oldline and newline 都是同一个renderType，但也有可能需要重新计算viewportRowmap
    - 举例：line10是标题，line11也是标题。cursor从10移动到11，这时line10也是需要重新展开渲染的
      。因为line10/line11不属于同一个segment渲染片

