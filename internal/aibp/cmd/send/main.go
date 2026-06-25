package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"net"
	"os"
	"time"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

func main() {
	name := flag.String("name", "", "目标 receiver 的 name（必填）")
	path := flag.String("file", "", "当前文件绝对路径（必填）")
	line := flag.Int("line", 0, "光标行（1-based）")
	col := flag.Int("col", 0, "光标列（1-based）")
	msg := flag.String("msg", "", "用户消息（有即「带消息」路径，无即「纯上下文」）")
	selText := flag.String("sel", "", "选区文本（可选）")
	selSLine := flag.Int("sel-sl", 0, "选区起点行")
	selSCol := flag.Int("sel-sc", 0, "选区起点列")
	selELine := flag.Int("sel-el", 0, "选区终点行")
	selECol := flag.Int("sel-ec", 0, "选区终点列")
	instance := flag.String("instance", "proto", "发送端实例标识")
	flag.Parse()
	if *name == "" || *path == "" {
		fmt.Fprintln(os.Stderr, "用法: send -name <receiver> -file <path> [-msg ...] [选区flags]")
		os.Exit(2)
	}

	payload := aibp.ContextPayload{
		Path:    *path,
		Cursor:  aibp.Position{Line: *line, Col: *col},
		Message: *msg,
	}
	if *selText != "" {
		payload.Selection = &aibp.Selection{
			Start: aibp.Position{Line: *selSLine, Col: *selSCol},
			End:   aibp.Position{Line: *selELine, Col: *selECol},
			Text:  *selText,
		}
	}
	payloadJSON, _ := json.Marshal(payload)

	env := aibp.Envelope{
		V: aibp.ProtocolMajor, Type: "context",
		Sender:  aibp.Sender{PID: os.Getpid(), Name: "microNeo", Instance: *instance},
		TS:      float64(time.Now().UnixNano()) / 1e9,
		Payload: payloadJSON,
	}

	// 找 socket：先 Discover 验活，再按 name 取（不能盲目用注册表里的 sock，可能已僵）
	receivers, err := aibp.Discover()
	if err != nil {
		fmt.Fprintln(os.Stderr, "discover:", err)
		os.Exit(1)
	}
	var sock string
	for _, r := range receivers {
		if r.Name == *name {
			sock = r.Socket
			break
		}
	}
	if sock == "" {
		fmt.Fprintf(os.Stderr, "找不到存活的 receiver: %s\n", *name)
		os.Exit(1)
	}

	// 说明-AIBP §4.2: connect → 写一行 JSON → close
	c, err := net.Dial("unix", sock)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dial:", err)
		os.Exit(1)
	}
	defer c.Close()
	line2, _ := env.MarshalLine()
	if _, err := c.Write(line2); err != nil {
		fmt.Fprintln(os.Stderr, "write:", err)
		os.Exit(1)
	}
	fmt.Printf("✓ 已发给 %s (%s:%d)\n", *name, *path, *line)
}
