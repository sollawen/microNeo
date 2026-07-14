## 场景1，现在有上下两个pane，paneA and paneB
我理解这两个pane其实是生活在一个tab0里面的

当其中一个pane是活跃pane的时候，比如说在paneA里面
- :big 就表示想把当前paneA，从当前tab0里移出去，扩大变成一个新的tab1，并且 tab1 就是活跃tab
- :small 表表示想把当前paneA，从当前tab0里移出去，缩小变成一个新的tab1，并且当前活跃tab0仍然是当前tab

## 场景2，现有两个tab

当前活跃的是tab0，里面只有一个paneA。不活跃的是tab1，里面只有一个paneB

当用户在paneA里面的时候
- :big 因为当前tab0只有一个pane，没法再扩大的，所以这个命令不起作用
- :small 当前tab0 变成有两个上下的pane
	- 上面的是paneA，因为paneA缩小了
	- 下面的是tab1里面的那个paneB
	- tab1就消失了，因为没有paneB了

## 场景3，现在有两个tab

当前活跃的是tab0，里面只有一个paneA。不活跃的是tab1，里面有2个pane，paneB and pancC

当用户在paneA里面的时候
- :big 因为当前tab0只有一个pane，没法再扩大的，所以这个命令不起作用
- :small 当前tab0 变成有两个上下的pane
	- 上面的是paneA，因为paneA缩小了
	- 下面的是tab1里面的某个pane
		- 一种选择是取tab1里面的第一个pane
		- 还一种选择是tab1里面的最新打开或修改过的pane
		- 这两个，我倾向于第二种，如果能实现最好
	- tab不消失，因为还剩下1个pane

## 场景4，现在只有一个tab，里面只有一个pane
用户在这个唯一的paneA里面
- :big 已经是最大了，没法再big了。不动
- :small 表示hsplit, paneA缩小留在上面，下面变成一个新的pane
	- 因为没有更多的tab了，所以下面的pane就是一个noName pane
