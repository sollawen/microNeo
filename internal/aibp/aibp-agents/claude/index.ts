import * as net from "node:net"
import * as fs from "node:fs"
import * as path from "node:path"
import * as os from "node:os"
import { fileURLToPath } from "node:url"

// 协议版本单一事实来源：package.json 的 aibp.protocol。
// startup 静态检测和运行时注册表声明都读同一字段，避免硬编码漂移。
const __dirname = path.dirname(fileURLToPath(import.meta.url))
const pkg = JSON.parse(fs.readFileSync(path.join(__dirname, "package.json"), "utf8"))
const PROTOCOL = pkg.aibp.protocol                       // 字符串，写注册表用
const PROTOCOL_MAJOR = Number(PROTOCOL.split("-").pop()) // 整数，校验信封用

// ============================================================
// 诊断日志（v1 调试期常开；链路对齐 aibp-opencode 的 log）
// 关键约束：stdout 专用于 Monitor 事件流，严禁 console.log 污染；诊断一律写文件。
// claude 子进程的 stderr 被父进程接管看不到，故写 /tmp/aibp-claude.log。
// ============================================================
const LOG_FILE = process.env.MNAB_LOG || "/tmp/aibp-claude.log"
let LOG_TAG = "boot"
function log(message: string, data?: unknown) {
  try {
    const ts = new Date().toISOString()
    const body = data === undefined
      ? ""
      : typeof data === "string" ? " " + data : " " + JSON.stringify(data)
    fs.appendFileSync(LOG_FILE, `${ts} [${LOG_TAG}] ${message}${body}\n`)
  } catch {}
}
log("===== module loaded =====", { pid: process.pid, protocol: PROTOCOL, major: PROTOCOL_MAJOR })

// D11 §4.2：默认 NATO 音标字母表前 15 个 A–O（去连字符满足 §4.3 字符约束）
const DEFAULT_NAMES_STR = "Alpha Bravo Charlie Delta Echo Foxtrot Golf Hotel India Juliet Kilo Lima Mike November Oscar"

// ============================================================
// 段 A：协议层（逐字复制自 aibp-pi/index.ts）
// ============================================================

// D11 §4.3：三步规范化（顺序：截断 10 字符 → 保留首次出现去重 → 字符过滤）
// 返回的池子已干净，allocateName 无需再判合法性。
export function normalizeNames(raw: string[]): string[] {
  // 步骤 1：截断每个 name 到前 10 字符（非 string 元素 → 空串，后续被去重/过滤清掉）
  const truncated = raw.map((n) => (typeof n === "string" ? n.slice(0, 10) : ""))
  // 步骤 2：保留首次出现顺序去重（不排序，保持"有序占用"语义）
  const seen = new Set<string>()
  const deduped: string[] = []
  for (const n of truncated) {
    if (n && !seen.has(n)) {
      seen.add(n)
      deduped.push(n)
    }
  }
  // 步骤 3：字符过滤——跳过含 / \0 : 空格 - 的 name（不报错，记日志）
  // `-` 是强约束：避免 name 自带 `-` 让 socket 文件名 `ai-<name>.sock`
  // 里的 `ai-` 分隔标记产生视觉歧义
  return deduped.filter((n) => {
    if (/[/\0: -]/.test(n)) {
      console.warn(`[aibp-daemon] skip illegal name: ${JSON.stringify(n)}`)
      return false
    }
    return true
  })
}

// D11 §4.5：加载名字池。
// 返回 string[]：已规范化的干净池子，进入正常分配。
// 返回 null：池子文件解析失败（notify 已内部完成，调用方应放弃注册，不要覆盖用户文件）。
export function loadNamePool(ctx: any): string[] | null {
  const xdg = process.env.XDG_CONFIG_HOME
  const configBase = xdg || path.join(os.homedir(), ".config")
  const poolFile = path.join(configBase, "aibp", "aibp-names.json")

  let raw: string[] | null = null

  if (fs.existsSync(poolFile)) {
    try {
      const parsed = JSON.parse(fs.readFileSync(poolFile, "utf8"))
      if (Array.isArray(parsed) && parsed.length > 0) {
        raw = parsed                     // 分支 A：合法非空数组 → 直接用
      }
      // 解析成功但是空数组 / 非数组 → raw 仍 null → 走分支 B 种子
    } catch {
      // 分支 C：解析失败 → notify + 返回 null（不覆盖用户文件，留给用户修）
      if (ctx?.ui?.notify) {
        ctx.ui.notify("⚠ aibp/aibp-names.json 格式错误，本次不接收消息", "warning")
      }
      return null
    }
  }

  // 分支 B：文件不存在 OR 内容空 → 等同"想重置"，原地种子默认值
  if (raw === null) {
    fs.mkdirSync(path.dirname(poolFile), { recursive: true, mode: 0o700 })
    fs.writeFileSync(poolFile, JSON.stringify(DEFAULT_NAMES_STR.split(" ")), { mode: 0o600 })
    raw = DEFAULT_NAMES_STR.split(" ")
  }

  return normalizeNames(raw)
}

// 辅助函数：获取注册目录
export function registryDir(): string {
  const override = process.env.MNAB_REG_DIR
  if (override) return override
  const base = process.env.XDG_RUNTIME_DIR
    || process.env.TMPDIR
    || "/tmp"
  return path.join(base, `aibp-${process.getuid?.() ?? 0}`)
}

// D11 §五 + §5.1：分配名字。
//
// connectionHandler 由调用方传入——确保 listen 抢锁创建的 server 就是最终处理消息的 server
// （不会出现"listen 成功但消息没人处理"的空 server）。
//
// 抢锁成功时：server 已 listen 并通过闭包赋值给外层变量。
// 池子耗尽（全部占用）→ 返回 null，由调用方 ctx.ui.notify。
export async function allocateName(
  names: string[],
  connectionHandler: (conn: net.Socket) => void,
): Promise<{ name: string; socketPath: string } | null> {
  let server: net.Server | null = null
  const dir = registryDir()
  fs.mkdirSync(dir, { recursive: true, mode: 0o700 })

  // 构建已占用集合 + 顺手 GC 僵尸注册（D11 §五步骤 3）
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
    const rid = m[1]                    // 裸名
    let pid: number | null = null
    try {
      const reg = JSON.parse(fs.readFileSync(path.join(dir, entry.name), "utf8"))
      if (typeof reg.pid === "number") pid = reg.pid
    } catch {
      continue                          // 注册文件本身坏：跳过（不 GC 用户数据）
    }
    let alive = false
    if (pid !== null) {
      try { process.kill(pid, 0); alive = true; } catch { alive = false; }
    }
    if (alive) {
      occupied.add(rid)
    } else {
      // PID 死 → 僵尸注册，顺手 GC（json + sock）
      try { fs.unlinkSync(path.join(dir, entry.name)); } catch {}
      try { fs.unlinkSync(path.join(dir, `ai-${rid}.sock`)); } catch {}
    }
  }

  // 尝试 listen 占用一个 socket 路径。
  // 成功 → server 通过闭包赋值；失败（EADDRINUSE 或其他）→ close 候选 server，返回 false。
  const tryListen = (sockPath: string): Promise<boolean> => {
    return new Promise((resolve) => {
      const s = net.createServer(connectionHandler)
      let settled = false
      const finish = (ok: boolean) => {
        if (settled) return
        settled = true
        if (ok) {
          // listen 成功 → 切换为运行态：移除抢锁阶段的 once error，换成持久日志 handler
          s.removeAllListeners("error")
          s.on("error", (err) => console.warn(`[aibp-daemon] server runtime error: ${err}`))
          server = s
          resolve(true)
        } else {
          s.close()
          resolve(false)
        }
      }
      s.once("listening", () => finish(true))
      s.once("error", (err: NodeJS.ErrnoException) => {
        if (err.code !== "EADDRINUSE") {
          console.warn(`[aibp-daemon] listen error on ${sockPath}: ${err}`)
        }
        finish(false)
      })
      s.listen(sockPath)
    })
  }

  // 按池子顺序尝试占用
  for (const n of names) {
    if (occupied.has(n)) continue
    const sockPath = path.join(dir, `ai-${n}.sock`)

    // D11 §5.1：listen 是原子锁——成功即占用
    if (await tryListen(sockPath)) {
      fs.chmodSync(sockPath, 0o600)
      return { name: n, socketPath: sockPath }
    }

    // listen 失败（EADDRINUSE）→ connect 试探判断真活 vs 僵尸 socket（200ms 超时）
    const alive = await new Promise<boolean>((resolve) => {
      const client = net.connect(sockPath)
      let done = false
      const finish = (ok: boolean) => {
        if (done) return
        done = true
        client.destroy()
        resolve(ok)
      }
      client.once("connect", () => finish(true))     // 真有接收端在 listen
      client.once("error", () => finish(false))      // ENOENT 等 → 僵尸/无主
      setTimeout(() => finish(false), 200)           // 超时 → 当僵尸处理
    })

    if (!alive) {
      // 僵尸 socket：删掉再 listen 一次
      try { fs.unlinkSync(sockPath); } catch {}
      if (await tryListen(sockPath)) {
        fs.chmodSync(sockPath, 0o600)
        return { name: n, socketPath: sockPath }
      }
    }
    // connect 成功（真活）OR 重 listen 失败 → 换下一个 name
  }

  return null                            // 池子耗尽
}

// ============================================================
// 段 B：formatText（输出格式化，从 aibp-pi 复制）
// ============================================================
export function formatText(p: any): string {
  const sel = p.selection
  const selText = sel?.text && sel.text.length > 0 ? sel.text : ""

  if (sel && selText) {
    // 有选区且文字未截断：内联 + 尖括号标签分隔（不用 @ 语法 → 不触发 LLM 读文件）
    const header = `<selection path="${p.path}" lines="${sel.start.line}-${sel.end.line}">`
    return p.message
      ? `${header}\n${selText}\n</selection>\n<user-input>\n${p.message}\n</user-input>`
      : `${header}\n${selText}\n</selection>`
  }

  // 无选区 / 选区文字被截断（超过 MaxSelectionLines）：走 @ 引用，让 LLM 自己读
  const focus = sel
    ? `line${sel.start.line}-${sel.end.line}`
    : `${p.cursor.line}`
  const base = `@${p.path} :line${focus}`
  return p.message ? `${base}\n\n${p.message}` : base
}

// ============================================================
// emitMonitorEvent：stdout 事件（替代 MCP notification）
// ============================================================
function emitMonitorEvent(p: any) {
  const text = formatText(p)
  const event = { type: "aibp-context", content: text }
  process.stdout.write(JSON.stringify(event) + "\n")
  log("emitted monitor event", { path: p.path, hasMsg: !!p.message })
}

// ============================================================
// 段 C：Unix socket listener
// ============================================================
const names = loadNamePool({ ui: { notify: (msg: string) => console.warn(`[aibp-daemon] ${msg}`) } })
if (!names) {
  console.error('[aibp-daemon] loadNamePool failed, exit')
  process.exit(1)
}

const connectionHandler = (conn: net.Socket) => {
  log("connection accepted")
  let buf = ''
  conn.on('data', chunk => {
    buf += chunk.toString('utf8')
    let nl
    while ((nl = buf.indexOf('\n')) >= 0) {
      handleLine(buf.slice(0, nl))
      buf = buf.slice(nl + 1)
    }
  })
  conn.on('error', (e) => log("connection error", { error: e.message }))
}

const alloc = await allocateName(names, connectionHandler)
if (!alloc) {
  console.error('[aibp-daemon] allocateName failed (pool exhausted or socket busy), exit')
  process.exit(1)
}
const { name, socketPath } = alloc

// 写 registry 文件
const regFile = path.join(registryDir(), `ai-${name}.json`)
fs.writeFileSync(regFile, JSON.stringify({
  name,
  agent: 'claude',
  pid: process.pid,
  transport: 'unix',
  socket: socketPath,
  protocol: PROTOCOL,
  startedAt: Math.floor(Date.now() / 1000),
  cwd: process.cwd(),
  labels: ['default'],
}))

LOG_TAG = name
console.error(`[aibp-daemon] listening as ${name} at ${socketPath}`)
log("listening", { name, socketPath, agent: "claude", pid: process.pid })

// ============================================================
// 段 D：投递（Monitor stdout 事件）
// ============================================================
function handleLine(line: string) {
  log("line received", { len: line.length, line })
  let env: any
  try { env = JSON.parse(line) } catch (e) { log("parse failed", { line, error: (e as Error).message }); return }
  log("envelope parsed", { v: env.v, type: env.type, hasPayload: !!env.payload })
  if (env.v !== PROTOCOL_MAJOR || env.type !== 'context') {
    log("envelope rejected (version/type mismatch)", { gotV: env.v, expectedV: PROTOCOL_MAJOR, gotType: env.type, expectedType: "context" })
    return
  }

  void onMessage(env)  // fire-and-forget
}

// v1：对齐 opencode，所有报文都触发
async function onMessage(env: any) {
  const p = env.payload
  log("onMessage", { path: p?.path, hasSelection: !!p?.selection, hasMessage: !!p?.message })
  emitMonitorEvent(p)
}

// 清理：Claude 关闭 stdin 时退出（触发 socket + registry 清理）
process.on('SIGPIPE', () => process.exit(0))
process.on('SIGTERM', () => process.exit(0))
process.on('SIGINT', () => process.exit(0))
// process.on('exit') 同步调用——在进程真正退出前触发（即使信号退出）
process.on('exit', () => {
  try { fs.unlinkSync(regFile) } catch {}
  try { fs.unlinkSync(socketPath) } catch {}
})
