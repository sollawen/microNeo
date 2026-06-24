/** @jsxImportSource @opentui/solid */
// aibp-opencode —— AIBP (AI Bridge Protocol) 在 opencode 上的接收端插件。
//
// 设计要点（见 docs/agent-comm/D19b-插件加载时机与形态反转.md）：
//   - 形态：TUI 插件（export default { id, tui }）。
//     TUI 插件在 App mount（主界面就绪）时立即加载，满足「启动即注册」需求。
//   - 协议层（registryDir / 名字池 / formatText / 分帧 / 版本校验）逐字复制 aibp-pi。
//   - 递送：最简版——只把消息发到 TUI 当前正在看的对话，不创建 session、不选 agent/model。
//   - 显示名字：toast 通知 + app_bottom slot 持久显示；清理用 api.lifecycle.onDispose。
//
// 注意：import type 在 Bun 运行时擦除，本文件零运行时外部依赖（仅 node:*）。
// JSX 运行时使用 opencode 环境已有的 @opentui/solid（peerDependency）。
// 调试日志：写 /tmp/aibp-opencode.log（append），tail -f 可实时观察。

import type { TuiPlugin, TuiPluginApi, TuiPluginModule, TuiSlotPlugin } from "@opencode-ai/plugin/tui"
import type { JSX } from "@opentui/solid"
import * as net from "node:net"
import * as fs from "node:fs"
import * as path from "node:path"
import * as os from "node:os"
import { fileURLToPath } from "node:url"

// ===== 诊断日志 =====
const LOG_FILE = "/tmp/aibp-opencode.log"
let LOG_TAG = "boot" // 分配名字前用 "boot"，注册成功后换成名字
function log(message: string, data?: unknown) {
  try {
    const ts = new Date().toISOString()
    const body =
      data === undefined ? "" : typeof data === "string" ? " " + data : " " + JSON.stringify(data)
    fs.appendFileSync(LOG_FILE, `${ts} [${LOG_TAG}] ${message}${body}\n`)
  } catch {}
}

log("===== module loaded =====")

// —— 协议版本单一事实来源：package.json 的 aibp.protocol ——
let PROTOCOL = "aibp-1"
let PROTOCOL_MAJOR = 1
try {
  const __dirname = path.dirname(fileURLToPath(import.meta.url))
  const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"))
  if (pkg?.aibp?.protocol) {
    PROTOCOL = pkg.aibp.protocol
    PROTOCOL_MAJOR = Number(PROTOCOL.split("-").pop())
  }
} catch {}
log("protocol detected", { protocol: PROTOCOL, major: PROTOCOL_MAJOR })

// D11 §4.2：默认 NATO 音标字母表前 15 个 A–O（去连字符满足 §4.3 字符约束）。
const DEFAULT_NAMES_STR =
  "Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel India Juliet Kilo Lima Mike November Oscar"

const tui: TuiPlugin = async (api: TuiPluginApi) => {
  const client = api.client // = OpencodeClient

  let server: net.Server | null = null
  let name = "",
    socketPath = "",
    regFile = "",
    slotRegId: string | undefined

  log("===== tui() invoked, plugin starting =====", { pid: process.pid, cwd: process.cwd() })

  function toast(message: string, variant: "info" | "warning" = "info") {
    log("toast", { message, variant })
    try {
      api.ui.toast({ message, variant })
    } catch (e) {
      log("toast failed", { error: (e as Error).message })
    }
  }

  // ===== 启动主体 =====

  const names = loadNamePool()
  log("name pool loaded", { ok: names !== null, count: names?.length, names })
  if (names === null) {
    toast("⚠ aibp/aibp-names.json 格式错误，本次不接收消息", "warning")
    log("name pool invalid, abort")
    return
  }

  const connectionHandler = (conn: net.Socket) => {
    log("connection accepted")
    let buf = ""
    conn.on("data", (chunk) => {
      const str = chunk.toString("utf8")
      log("data chunk", { len: chunk.length, preview: str.slice(0, 200) })
      buf += str
      let nl
      while ((nl = buf.indexOf("\n")) >= 0) {
        handleLine(buf.slice(0, nl))
        buf = buf.slice(nl + 1)
      }
    })
    conn.on("error", (e) => log("connection error", { error: e.message }))
    conn.on("close", () => log("connection closed"))
  }

  const got = await allocateName(names, connectionHandler)
  if (got === null) {
    toast("⚠ aibp 名字池已满，本次不接收消息", "warning")
    log("name allocation exhausted, abort")
    return
  }
  name = got.name
  socketPath = got.socketPath
  LOG_TAG = name // 后续日志带名字

  regFile = path.join(registryDir(), `ai-${name}.json`)
  fs.writeFileSync(
    regFile,
    JSON.stringify({
      name,
      pid: process.pid,
      transport: "unix",
      socket: socketPath,
      protocol: PROTOCOL,
      startedAt: Math.floor(Date.now() / 1000),
      cwd: process.cwd(),
      labels: ["default"],
    }),
  )
  log("registry file written", { regFile, name, pid: process.pid, socketPath })

  log("===== ready =====", { name })

  // ===== 注册 app_bottom slot 持久显示名字 =====
  try {
    const slotPlugin: TuiSlotPlugin = {
      order: 1000, // 靠后显示，避免遮挡其他内容
      slots: {
        app_bottom(ctx) {
          return <text>● {name}</text>
        },
      },
    }
    slotRegId = api.slots.register(slotPlugin)
    log("app_bottom slot registered", { slotRegId })
  } catch (e) {
    log("slot registration failed", { error: (e as Error).message })
    toast("⚠ aibp 已就绪，但底部显示失败", "warning")
  }

  const cleanup = () => {
    log("cleanup start", { name, regFile, socketPath, slotRegId })
    try {
      server?.close()
    } catch {}
    // opencode 自动清理 slot，无需手动 unregister
    try {
      fs.unlinkSync(regFile)
    } catch {}
    try {
      fs.unlinkSync(socketPath)
    } catch {}
    log("cleanup done")
  }
  api.lifecycle.onDispose(cleanup)

  // ===== 报文处理（递送）=====

  function handleLine(line: string) {
    log("line received", { len: line.length, line })
    let env: any
    try {
      env = JSON.parse(line)
    } catch (e) {
      log("parse failed", { line, error: (e as Error).message })
      return
    }
    log("envelope parsed", { v: env.v, type: env.type, hasPayload: !!env.payload })
    if (env.v !== PROTOCOL_MAJOR || env.type !== "context") {
      log("envelope rejected (version/type mismatch)", {
        gotV: env.v,
        expectedV: PROTOCOL_MAJOR,
        gotType: env.type,
        expectedType: "context",
      })
      return
    }
    void onMessage(env)
  }

  // 递送策略（最简版）：只发到 TUI 当前正在看的对话，不创建 session、不选 agent/model。
  async function onMessage(env: any) {
    const p = env.payload
    log("onMessage", {
      path: p?.path,
      hasSelection: !!p?.selection,
      selectionLen: p?.selection?.text?.length,
      selectionStart: p?.selection?.start,
      selectionEnd: p?.selection?.end,
      cursor: p?.cursor,
      hasMessage: !!p?.message,
      message: p?.message,
    })

    const text = formatText(p)
    log("formatText output (FULL TEXT SENT TO LLM)", { text })

    try {
      const route = api.route.current as any
      log("current route", {
        name: route?.name,
        params: route?.params,
      })

      if (route?.name !== "session" || typeof route?.params?.sessionID !== "string") {
        log("no active session in TUI, skipping delivery")
        toast("⚠ 请先在 opencode 打开一个对话", "warning")
        return
      }

      const sessionID = route.params.sessionID
      // v2 SDK 顶层参数风格（非 v1 的 {path, body}）。
      // 插件拿到的 api.client 是 @opencode-ai/sdk/v2 的 OpencodeClient，
      // session.prompt 签名是 ({ sessionID, parts, agent?, ... })，URL=/session/{sessionID}/message。
      // opencode 自己（prompt/index.tsx:1090）就是这么调的。fire-and-forget：不 await，
      // TUI 通过 session 订阅渲染流式响应。
      log("session.prompt calling", { sessionID })

      client.session
        .prompt({
          sessionID,
          parts: [{ type: "text", text }],
        })
        .then((res: any) => {
          log("session.prompt resolved", {
            status: res?.status,
            hasData: !!res?.data,
            hasError: !!res?.error,
            error: res?.error,
          })
        })
        .catch((e: any) => {
          log("session.prompt REJECTED", { message: e?.message, stack: e?.stack, name: e?.name })
        })

      toast("microNeo: 已发送到当前对话", "info")
    } catch (e) {
      const err = e as Error
      log("onMessage ERROR", { message: err.message, stack: err.stack, name: err.name })
      toast(`⚠ aibp 递送失败: ${err.message}`, "warning")
    }
  }

  // ===== 以下 ⟨复制 aibp-pi⟩ =====

  function normalizeNames(raw: string[]): string[] {
    const truncated = raw.map((n) => (typeof n === "string" ? n.slice(0, 10) : ""))
    const seen = new Set<string>()
    const deduped: string[] = []
    for (const n of truncated) {
      if (n && !seen.has(n)) {
        seen.add(n)
        deduped.push(n)
      }
    }
    return deduped.filter((n) => {
      if (/[/\0: -]/.test(n)) {
        log("skip illegal name", { name: n })
        return false
      }
      return true
    })
  }

  function loadNamePool(): string[] | null {
    const xdg = process.env.XDG_CONFIG_HOME
    const configBase = xdg || path.join(os.homedir(), ".config")
    const poolFile = path.join(configBase, "aibp", "aibp-names.json")
    log("loading name pool", { poolFile })

    let raw: string[] | null = null

    if (fs.existsSync(poolFile)) {
      try {
        const parsed = JSON.parse(fs.readFileSync(poolFile, "utf8"))
        if (Array.isArray(parsed) && parsed.length > 0) {
          raw = parsed
        }
      } catch {
        toast("⚠ aibp/aibp-names.json 格式错误，本次不接收消息", "warning")
        log("name pool file corrupt")
        return null
      }
    }

    if (raw === null) {
      fs.mkdirSync(path.dirname(poolFile), { recursive: true, mode: 0o700 })
      fs.writeFileSync(poolFile, JSON.stringify(DEFAULT_NAMES_STR.split(" ")), { mode: 0o600 })
      raw = DEFAULT_NAMES_STR.split(" ")
      log("seeded default name pool")
    }

    return normalizeNames(raw)
  }

  async function allocateName(
    names: string[],
    connectionHandler: (conn: net.Socket) => void,
  ): Promise<{ name: string; socketPath: string } | null> {
    const dir = registryDir()
    fs.mkdirSync(dir, { recursive: true, mode: 0o700 })
    log("allocating name", { dir, candidates: names })

    const occupied = new Set<string>()
    let entries: fs.Dirent[]
    try {
      entries = fs.readdirSync(dir, { withFileTypes: true })
    } catch {
      entries = []
    }
    for (const entry of entries) {
      if (!entry.isFile()) continue
      const m = entry.name.match(/^ai-(.+)\.json$/)
      if (!m) continue
      const rid = m[1]
      let pid: number | null = null
      try {
        const reg = JSON.parse(fs.readFileSync(path.join(dir, entry.name), "utf8"))
        if (typeof reg.pid === "number") pid = reg.pid
      } catch {
        continue
      }
      let alive = false
      if (pid !== null) {
        try {
          process.kill(pid, 0)
          alive = true
        } catch {
          alive = false
        }
      }
      if (alive) {
        occupied.add(rid)
      } else {
        log("gc zombie registry", { rid, pid })
        try {
          fs.unlinkSync(path.join(dir, entry.name))
        } catch {}
        try {
          fs.unlinkSync(path.join(dir, `ai-${rid}.sock`))
        } catch {}
      }
    }
    log("scan done", { occupied: [...occupied] })

    const tryListen = (sockPath: string): Promise<boolean> => {
      return new Promise((resolve) => {
        const s = net.createServer(connectionHandler)
        let settled = false
        const finish = (ok: boolean) => {
          if (settled) return
          settled = true
          if (ok) {
            s.removeAllListeners("error")
            s.on("error", (err) => log("server runtime error", { error: err.message }))
            server = s
            resolve(true)
          } else {
            s.close()
            resolve(false)
          }
        }
        s.once("listening", () => finish(true))
        s.once("error", (err: NodeJS.ErrnoException) => {
          log("listen error", { sockPath, code: err.code, message: err.message })
          finish(false)
        })
        s.listen(sockPath)
      })
    }

    for (const n of names) {
      if (occupied.has(n)) {
        log("name occupied, skip", { name: n })
        continue
      }
      const sockPath = path.join(dir, `ai-${n}.sock`)
      log("try listen", { name: n, sockPath })

      if (await tryListen(sockPath)) {
        fs.chmodSync(sockPath, 0o600)
        log("listen ok, allocated", { name: n, sockPath })
        return { name: n, socketPath: sockPath }
      }

      const alive = await new Promise<boolean>((resolve) => {
        const probe = net.connect(sockPath)
        let done = false
        const finish = (ok: boolean) => {
          if (done) return
          done = true
          probe.destroy()
          resolve(ok)
        }
        probe.once("connect", () => finish(true))
        probe.once("error", () => finish(false))
        setTimeout(() => finish(false), 200)
      })
      log("probe result", { name: n, alive })

      if (!alive) {
        try {
          fs.unlinkSync(sockPath)
        } catch {}
        if (await tryListen(sockPath)) {
          fs.chmodSync(sockPath, 0o600)
          log("listen ok after gc, allocated", { name: n, sockPath })
          return { name: n, socketPath: sockPath }
        }
      }
    }

    return null
  }

  function registryDir(): string {
    const override = process.env.MNAB_REG_DIR
    if (override) return override
    const base = process.env.XDG_RUNTIME_DIR || process.env.TMPDIR || "/tmp"
    return path.join(base, `microneo-agent-bridge-${process.getuid?.() ?? 0}`)
  }

  function formatText(p: any): string {
    const sel = p.selection
    const selText = sel?.text && sel.text.length > 0 ? sel.text : ""

    if (sel && selText) {
      const header = `<selection: ${p.path} lines ${sel.start.line}-${sel.end.line}>`
      return p.message ? `${header}\n\n${selText}\n\n<user input>\n\n${p.message}` : `${header}\n\n${selText}`
    }

    const focus = sel ? `line${sel.start.line}-${sel.end.line}` : `${p.cursor.line}`
    const base = `@${p.path} :line${focus}`
    return p.message ? `${base}\n\n${p.message}` : base
  }
}

const plugin: TuiPluginModule = {
  id: "aibp-opencode", // 文件式加载必需
  tui,
}

export default plugin
