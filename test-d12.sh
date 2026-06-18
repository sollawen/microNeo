#!/bin/bash
# D12 测试矩阵执行脚本
# 使用方法：./test-d12.sh [T1-T9]

set -e

# 前置：每次测试前重置环境
reset_env() {
    killall pi 2>/dev/null || true
    sleep 0.5
}

# T1：Discover err
test_T1() {
    echo "=== T1: Discover err ==="
    reset_env
    MNAB_REG_DIR=/nonexistent ./microneo &
    MICRO_PID=$!
    sleep 1

    # 预期：InfoBar 显示 "✗ discover error: ..."
    echo "手动验证：在 microNeo 中按 Alt-Enter，应看到 '✗ discover error'"

    kill $MICRO_PID 2>/dev/null || true
}

# T2：0 receiver
test_T2() {
    echo "=== T2: 0 receiver ==="
    reset_env
    # 不启动任何 pi
    ./microneo &
    MICRO_PID=$!
    sleep 1

    echo "手动验证：在 microNeo 中按 Alt-Enter，应看到 '✗ no receiver found'"

    kill $MICRO_PID 2>/dev/null || true
}

# T3：1 receiver
test_T3() {
    echo "=== T3: 1 receiver ==="
    reset_env
    # 启动 1 个 pi (Alpha)
    pi &
    sleep 1

    echo "手动验证：在 microNeo 中按 Alt-Enter → notePane 开（边框带 '→ pi-Alpha'）→ 写一行 → Alt-Enter → Alpha 收到消息"

    killall pi 2>/dev/null || true
}

# T4：2+ 初次选择
test_T4() {
    echo "=== T4: 2+ 初次选择 ==="
    reset_env
    # 启动 2 个 pi (Alpha, Bravo)
    pi &
    sleep 0.5
    pi &
    sleep 1

    echo "手动验证：在 microNeo 中按 Alt-Enter → SelectPane 弹（含 Alpha/Bravo）→ ↓ → Enter → 选 Bravo → notePane 开带 '→ pi-Bravo' → 发送 → Bravo 收到（非 Alpha）"

    killall pi 2>/dev/null || true
}

# T5：缓存命中
test_T5() {
    echo "=== T5: 缓存命中 ==="
    reset_env
    # 启动 2 个 pi
    pi &
    sleep 0.5
    pi &
    sleep 1

    echo "手动验证：T4 后，两 pi 仍在，再次 Alt-Enter → SelectPane 不弹，notePane 直接开带 '→ pi-Bravo'"

    killall pi 2>/dev/null || true
}

# T6：缓存失效
test_T6() {
    echo "=== T6: 缓存失效 ==="
    reset_env
    # 先启动 2 个 pi，T4 测试后 kill Bravo
    pi &
    sleep 0.5
    pi &
    sleep 1

    echo "手动验证：T4 后 kill Bravo（只剩 Alpha）→ Alt-Enter → SelectPane 弹（缓存未命中），可选 Alpha"

    killall pi 2>/dev/null || true
}

# T7：Esc 清零
test_T7() {
    echo "=== T7: Esc 清零 ==="
    reset_env
    pi &
    sleep 0.5
    pi &
    sleep 1

    echo "手动验证：Alt-Enter → SelectPane 弹 → 按 Esc → InfoBar '✗ 已取消'，selectedReceiver 清零"

    killall pi 2>/dev/null || true
}

# T8：Esc 后再开
test_T8() {
    echo "=== T8: Esc 后再开 ==="
    reset_env
    pi &
    sleep 0.5
    pi &
    sleep 1

    echo "手动验证：T7 后，两 pi 仍在，再次 Alt-Enter → 缓存空 → 弹 SelectPane（验证清零生效）"

    killall pi 2>/dev/null || true
}

# T9：SelectPane wrap
test_T9() {
    echo "=== T9: SelectPane wrap ==="
    reset_env
    pi &
    sleep 0.5
    pi &
    sleep 0.5
    pi &
    sleep 1

    echo "手动验证：Alt-Enter → 按 ↓ 多次 → 观察 wrap-around：到底后跳回首项"

    killall pi 2>/dev/null || true
}

# 主入口
case "${1:-all}" in
    T1) test_T1 ;;
    T2) test_T2 ;;
    T3) test_T3 ;;
    T4) test_T4 ;;
    T5) test_T5 ;;
    T6) test_T6 ;;
    T7) test_T7 ;;
    T8) test_T8 ;;
    T9) test_T9 ;;
    all)
        echo "运行所有测试（需要手动验证每个场景）"
        echo "由于测试需要交互式验证，请逐个运行："
        echo "  ./test-d12.sh T1"
        echo "  ./test-d12.sh T2"
        echo "  ..."
        ;;
    *)
        echo "用法：$0 [T1-T9]"
        exit 1
        ;;
esac
