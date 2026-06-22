package main

import (
	"encoding/json"
	"fmt"
	"os"

	"github.com/micro-editor/micro/v2/internal/aibp"
)

func main() {
	receivers, err := aibp.Discover()
	if err != nil {
		fmt.Fprintln(os.Stderr, "discover:", err)
		os.Exit(1)
	}
	if len(receivers) == 0 {
		fmt.Println("(无存活 receiver）")
		return
	}
	// 人类可读 + --json 机器可读（send 内部不用此，自己直接调 aibp.Discover）
	if jsonOut := len(os.Args) > 1 && os.Args[1] == "--json"; jsonOut {
		b, _ := json.MarshalIndent(receivers, "", "  ")
		fmt.Println(string(b))
		return
	}
	for _, r := range receivers {
		fmt.Printf("%-20s pid=%-6d sock=%s\n", r.Name, r.PID, r.Socket)
	}
}
