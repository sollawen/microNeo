package eabp

import (
	"encoding/json"
)

const Protocol = "eabp-1"

// Envelope — 说明-EABP §5.1 公共信封
type Envelope struct {
	V       int             `json:"v"`       // 主版本，当前=1
	Type    string          `json:"type"`    // "context"（v1仅）/ "bye"(预留)
	Sender  Sender          `json:"sender"`
	TS      float64        `json:"ts"`      // Unix 浮点秒
	Payload json.RawMessage `json:"payload"` // 原样透传；调用方按 Type 自行反序列化
}

type Sender struct {
	PID      int    `json:"pid"`
	Name     string `json:"name"`     // "microNeo"
	Instance string `json:"instance"` // 窗口/实例标识
}

// Position — 说明-EABP §5.3，被 Cursor / Selection.Start / Selection.End 复用。1-based（行从 1 起、列从 1 起）
type Position struct {
	Line int `json:"line"`
	Col  int `json:"col"`
}

// ContextPayload — 说明-EABP §5.3 `context` 报文 payload
type ContextPayload struct {
	Path         string     `json:"path"`
	Cursor       Position   `json:"cursor"`
	Selection    *Selection `json:"selection,omitempty"`    // 无选区则省略（不是 null）
	Message      string     `json:"message,omitempty"`      // 有无决定递送路径（说明-EABP §六）
	VisibleLines string     `json:"visible_lines,omitempty"`
}

type Selection struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
	Text  string   `json:"text,omitempty"`
}

// MarshalLine 序列化为单行 JSON + \n（说明-EABP §4.3 分帧）。调用方写入 socket 后关闭连接。
func (e *Envelope) MarshalLine() ([]byte, error) {
	b, err := json.Marshal(e)
	if err != nil {
		return nil, err
	}
	return append(b, '\n'), nil
}