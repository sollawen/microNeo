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

## 方案B，彻底重构

每次scroll up/down 的时候，都先渲染，再scroll

也就是说，当光标上下移动/或是用户输入了什么内容导致buffer变化的时候
- 先把viewport 从头开始 render, 把render后的数据(就是现在代码里render之后直接送进viewport去显示的的那堆数据)先暂存起来，不送进display
- 根据render后的数据，判断是否需要scroll, 需要scroll 几个row
- 把暂存的render后的数据(已经render过了，不需要重新render了)直接送给display，并告诉从是从第几个row开始显示的。于是display就从这些已经render好的数据里，找到start row, 并把后面的数据直接送进viewport显示出来

这样就彻底解决了row/line vs viewportRow 的计算和对应关系

换个说法：
- micro原生的时序是：
	- 用户做了些操作
	- 在 displayBuffer里面进行渲染，然后直接送进 display层
- 方案B的时序是
	- 用户做了些操作
	- 马上渲染但不显示，渲染后的用来显示的数据，保存在 renderedMap里面
	- 做一些必要的判断和处理
	- displayBufferMD()，里面不再render了，直接把renderedMap送进display层
- 这个改动不是特别为scroll服务的，而且根本改变
	- 用户各种对文件的改动，都会进行预渲染 
	- 预渲染是从buffer.startLine开始的，往下面完成整段的segment渲染。就是说如果遇到了一个代码块/表格，那么reder出来的是完整的segment，储存在renderedMap里
	- displayBufferMD()永远都是不渲染，只是把renderdMap中的准确的row送去显示

**本文档后续章节聚焦方案B。**

# 三、时序分析
	
## micro的原生时序

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

因此，方案B需要引入一个 renderedMap（实现上命名为 `bigViewportMap`）。
- 各render计算出来的结果并不直接送去displayer然后就扔掉了。而是以renderedMap为假装的显示viewport，把render后的数据送到这个map里面
- 所以，当各render运算完成后，这个map其实就是一个比当前viewport略长一点的bigViewportMap
	- 可以在现有的 viewportRowmap基础上改造一下，为每一个row，增加更多的字段
- 当displayBufferMD()的时候，就可以不用再render第二次了，
	- 直接把renderedMap拿过来，
	- 找到startRow，
	- 从startRow开始把每个row的数据直接填在displayer里面就行了

# 四、方案B
## 出发点
把渲染和显示分开
- 创建一个screenBuffer
- display(startLine)以前是渲染时直接填进viewport，现在改成渲染时填进screenBuffer
- screen.Show()之前，把screenBuffer里的数据直接填进真正的显示区域

## 主循环：
```
	文件刚打开时，displayToBuffer(0,true)
	while true {
	    screen.ShowBuffer(startLine)
	    event = <-screen.Events
	    handleEvent(event) {
	        case A: 状态变更（编辑 / 上下左右移动光标）{
	            改 buffer内容 or 光标所在的line
	            displayToBuffer(startLine, showCursor=true)  	
	            	//这时的startLine没有变，还是原来的值
	            	//true的时候，光标所在行要用native
	            startLine = Relocate(startLine)    // 计算要保证光标被渲染进viewport，startLine应该是多少
	        }
	        case B: 纯视口（scroll / editMode切换）{
	            光标是不动的
	            if scroll then 直接粗暴的估算 startLine
	            displayToBuffer(startLine, showCursor=false)    
	            	// false：光标所在行使用render
	        case C: 光标大幅移动且需要显示光标所在行 (page/center/start/end/goto)
	            光标移动到新的buffer.line
	            直接估算 startLine1
	            displayToBuffer(startLine1, showCursor=true)  	
	            startLine= Relocate(startLine1)    // 精算startLine，保证cursor所在行显示
	        	
	        }
	    }
	}
```

## displayToBuffer():
签名

```
func displayToBuffer(startLine, showCursor bool) {
    visibleStart = startLine  //就是说，要从startLine开始计算
	// 这里不需要 visibleEnd, 因为暂时不知道应该在哪里结束    
```

原来的主循环:
```
    for seg in segments:
           if editMode and hasCursorInside(seg, cursors):
               vY = renderSegmentNative(seg, vY)              # 编辑模式+光标在此段
           else:                                              #   → 回退原生(显示 # | 等原始字符)
               vY = renderSegmentMD(seg, vY)                  # 阅读模式 → MD渲染管线
```

因为现在不是固定长度的segments了，需要动态判断退出循环的条件，所以要修改成：
```
	cursorRow=-1
	cursorShowed=false
	
	while true {
	    if 光标所在行 and editMode and showCursor then {
	        renderSegmentNative() -> screenBuffer
	        cursorShowed = true
	        cursorRow = 当前最后一个 row 的序号
	    } else {
	        renderSegmentNative() -> screenBuffer
	        cursorShowed = true
	        cursorRow = 当前最后一个 row 的序号
	    }

        // 停止条件：
        canBreak=true    
        if not 旧光标所在段已整段渲染完整 then canBreak=false
        	// 由上面"segment 全部计算完成"自动保证（概念清晰，实现免费）

        if showCursor==true {
	        if not cursorShowed  then canBreak=false
            if not 光标所在行之后再渲染够 scrollMargin 行 then canBreak=false
        }
        //  if showCursor==false 其实就不用管新光标所在行是否已经渲染了
        //     只需要判断有没有把viewport都渲染完全就可以了
		if bigViewportMap的最底下的row <= viewport 的底边 then canBreak=false 
         
        if canBreak then break	
    }

	// 填充剩余空白(buf 内容不够长)
	while vY < bufHeight {
	    drawEmptyLine(vY)                                  # 无行号 + 默认背景
	    vY++	
	}
```

**Relocate()**:
```
func Relocate(oldStartLine) {
    newStartLine = oldStartLine

    根据 cursor.line 和 screenBuffer，计算光标所在的 viewport 里的 row
    if cursor.row小于安全区的上边 {
        // 顶部分支：只服务单步↑。大跨度上移走 case B（scroll/page）或 GotoLoc 的 Scroll 预定位，不依赖此处
        newStartLine = oldStartLine - 1
        return newStartLine
    }

    if 光标大于安全区的下边 {
        根据 cursor.line 和 screenBuffer，计算把光标所在的 row 显示在安全区最下边及scrollMargin的时候，newStartLine 应该是多少
        newStartLine = 刚才的计算结果
    }
    return newStartLine
}
```

