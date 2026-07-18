package config

import (
	"fmt"
	"strings"

	"github.com/micro-editor/tcell/v2"
)

// MetaToAlt 把 Meta 修饰键折叠成 Alt。终端层 Meta 与 Alt 常等价，
// 统一成 Alt 才能和 tcell.KeyNames / bindings.json 的键名约定对齐。
func MetaToAlt(mod tcell.ModMask) tcell.ModMask {
	if mod&tcell.ModMeta != 0 {
		mod &= ^tcell.ModMeta
		mod |= tcell.ModAlt
	}
	return mod
}

// KeyNameOf 把按键三要素翻译成标准键名（与 bindings.json 的 key 一致）。
// mod 会先经 MetaToAlt 规范化，因此传原始或已规范化的 mod 都正确（幂等）。
func KeyNameOf(code tcell.Key, mod tcell.ModMask, r rune) string {
	mod = MetaToAlt(mod)
	m := []string{}
	if mod&tcell.ModShift != 0 {
		m = append(m, "Shift")
	}
	if mod&tcell.ModAlt != 0 {
		m = append(m, "Alt")
	}
	if mod&tcell.ModMeta != 0 {
		m = append(m, "Meta")
	}
	if mod&tcell.ModCtrl != 0 {
		m = append(m, "Ctrl")
	}

	s, ok := tcell.KeyNames[code]
	if !ok {
		if code == tcell.KeyRune {
			s = string(r)
		} else {
			s = fmt.Sprintf("Key[%d]", code)
		}
	}
	if len(m) != 0 {
		if mod&tcell.ModCtrl != 0 && strings.HasPrefix(s, "Ctrl-") {
			s = s[5:]
			if len(s) == 1 {
				s = strings.ToLower(s)
			}
		}
		return fmt.Sprintf("%s-%s", strings.Join(m, "-"), s)
	}
	return s
}

// KeyName 从 tcell 按键事件翻译出标准键名。叶子包处理键盘的统一入口。
func KeyName(e *tcell.EventKey) string {
	code := e.Key()
	r := rune(0)
	if code == tcell.KeyRune {
		r = e.Rune()
	}
	return KeyNameOf(code, e.Modifiers(), r)
}
