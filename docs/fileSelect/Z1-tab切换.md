## 场景1，现在有上下两个pane，paneA and paneB
我理解这两个pane其实是生活在一个tab0里面的

当其中一个pane是活跃pane的时候，比如说在paneA里面
- 用户按 "alt-+" 就表示想把当前paneA，从当前tab0里移出去，扩大变成一个新的tab1，并且 tab1 就是活跃tab
- 用户按 "alt-_" 表表示想把当前paneA，从当前tab0里移出去，缩小变成一个新的tab1，并且当前活跃tab0仍然是当前tab

## 场景2，现有两个tab

当前活跃的是tab0，里面只有一个paneA

当用户在paneA里面的时候
- "alt-+", 因为当前tab只有一个pane，没法再扩大的，所以这个命令不起作用
- "alt-_", 当前tab0 变成有两个上下的pane
	- 上面的是paneA，因为paneA缩小了
	- 下面的是tab1里面的那个pane
