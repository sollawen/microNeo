# What breaks when a line editor stops being linear

> Status: English, ready to ship
> Channel: dev.to (primary), HN/Reddit comment reuse
> Length: ~1850 words
> Selected title: B (from 3 candidates, decided 2026-06-19)
> Rejected: A ("25k-star" feels like marketing), C (too short for feed), "How to transform..." (feels like tutorial + clickbait "?" + loses author voice)

---

## Body

Every terminal editor you've ever used — vim, nano, Micro, pico — is fundamentally the same thing: a **line editor**. One line in the buffer, one line on the screen. A 520-line rendering function copies each character to the screen verbatim, and the only thing that ever changes is color. Micro earned its 25k stars on that one simple assumption.

That assumption is correct. All source code, all config files, get processed this way. Generations of programmers have worked with files this way.

But then vibe coding arrived, and I found myself writing less and less code — and reading more and more AI-written Markdown. Suddenly every editor in my terminal felt wrong. I wanted Markdown to render and edit in the same window. Which meant breaking the 1:1 assumption the entire codebase was built on.

I tolerated it for a few months. Then I couldn't. So I started building it myself — based on Micro, turning a line editor into a rich-format editor.

---

### 1. What a line editor actually is (and why the assumption is valuable)

Here's how Micro works.

Its core render function is called `displayBuffer()`, in `bufwindow.go`, ~520 lines. Strip out all the bells and whistles — brace matching, whitespace display, selection highlight, softwrap — and the skeleton is just two nested loops:

```
for each visible buffer line {
    draw the line number
    for each character in that line {
        get the syntax highlight color
        screen.SetContent(x, y, rune, style)
    }
}
```

**Buffer line N is screen line N.** (Softwrap can split one line into several, but it never breaks the property "every screen line traces back to exactly one buffer line.")

Why is that assumption so valuable?

Because it makes **scrolling** almost free. Micro's scroll system revolves around a struct called `SLoc`, which records "which buffer line is at the top of the window." Scrolling one line is just `SLoc.Line += 1`. Cursor positioning, PageUp/PageDown, mouse clicks — everything sits on top of the same "buffer line and screen line correspond one-to-one" premise.

The design is clean, fast, easy to maintain. Micro has run on it for a decade.

---

### 2. The crash site: how the 1:1 assumption dies

I open a 3-line Markdown table:

```
| name | age |
|------|-----|
| ada  | 36  |
```

I don't want to see those silly pipe characters. I want a real table: borders, column alignment, a bold header.

Here's the problem. A 3-line buffer renders into 5 screen lines (top border, header row, separator, data row, bottom border). **After buffer line 3, two extra screen lines appear that simply don't exist in the buffer.** I have to draw them out of thin air.

That's the moment 1:1 dies. And it's not just tables:

- `**bold**` renders as **bold** — 7 characters in the buffer, 4 on the screen
- `# heading` renders as a large heading — the `#` vanishes from the screen
- ```` ``` ```` code fences — become box lines, the fences themselves disappear

Any one of these alone is easy. The hard part: **Micro's scrolling, cursor, click handling, selection — the entire interaction layer — assumes "screen line Y can be reverse-mapped to buffer line X."** Now that reverse mapping is broken.

The really nasty one is **multi-line structures** like tables. You can't know how wide a column should be by looking at a single line — you have to scan the entire table. Which means rendering can no longer be "line-by-line, independent." Something has to treat cross-line structures as a whole.

---

### 3. The core decision: wedge a Segment layer between buffer and screen

The first — and most important — architectural call I made: **don't touch the buffer, and don't touch the tcell screen. Add a layer in between.**

I call this layer **Segment**. Here's what the code looks like (`internal/md/md.go`, simplified):

```go
type Segment struct {
    BufStartLine int       // which buffer lines this segment covers
    BufEndLine   int
    Render func(seg Segment, width int, cfg MDConfig) *RenderedSegment
}

type Cell struct {
    Rune         rune
    Style        tcell.Style
    BufLine      int   // which buffer line this screen char maps to; decorative = -1
    BufX         int   // rune offset within that buffer line
    IsDecorative bool   // true = decorative char (border line); clicks ignored
}
```

Two design decisions, each solving a different class of problem:

**Design 1: every buffer line belongs to some Segment.**

Segments aren't only for tables and code blocks. **A heading line is a 1-line Segment; a plain paragraph line is also a 1-line Segment.** That keeps the render pipeline branch-free:

```
for each visible line {
    find the Segment this line belongs to
    let the Segment handle the output
}
```

The only difference between a table Segment and a paragraph Segment is how many buffer lines it covers and which render function it holds. The pipeline is uniform. No `if isTable`.

**Design 2: split "classification" and "rendering" into two fully independent stages.**

This one I only figured out after hitting a wall. Initially I wanted the render function to compute row heights as it rendered. Then I ran into a wall:

Micro's `Scroll()` and `Relocate()` **are called before `displayBuffer()`.** They have to decide "is it valid to scroll here," which requires knowing how many screen lines each segment occupies. But row heights only get computed once rendering reaches that segment.

Timing deadlock.

The fix: pull "this is a table, covers these lines, holds this render function" — the info that **depends only on buffer content, never on screen width** — out into a separate stage called `DetectSegments()`. It runs the moment the buffer changes, and stores the result. The render stage just reads that result.

```
buffer change ──► DetectSegments() ──► stored (screen-width-independent)
                                            │
key/scroll ──► Scroll/Relocate (reads row heights) ──► render (layouts by width) ──► screen
```

Detect knows nothing about screen width. Render only reads what detect produced. The two stages are decoupled. The deadlock is gone.

---

### 4. With segments in place, you need a pack of renderers

A Segment is just a data structure — it declares "this stretch of buffer is one unit," but says nothing about "how to translate that stretch into screen characters." That translation is the job of a pack of ordinary functions called **renders**.

Each render looks like this (real signature, `internal/md/render_table.go`):

```go
func RenderTable(seg Segment, width int, cfg MDConfig) *RenderedSegment
```

Input: a Segment (knows which buffer lines it covers), the current screen width, config. Output: a `RenderedSegment` — a list of `RenderedRow`s, each a list of `Cell`s.

I built seven renders:

| render | covers | example |
|---|---|---|
| `RenderHeading` | single `# heading` | `# Hello` → large "Hello" |
| `RenderHR` | single `---` | `---` → a horizontal rule |
| `RenderBlockquote` | consecutive `>` lines | `> quote` → quote block with a vertical bar prefix |
| `RenderList` | consecutive `- ` / `1.` lines | list items |
| `RenderCodeBlock` | fenced multi-line | a code block with box lines |
| `RenderTable` | consecutive `\|` lines | a bordered, column-aligned table |
| `RenderParagraph` | fallback, single line | a normal paragraph; handles inline bold/italic |

One thing to notice: **these seven renders don't know about each other.** `RenderTable` has no idea `RenderCodeBlock` exists. They also don't know where the cursor is, whether the screen is scrolling, or whether the user is in edit mode. They do exactly one thing: give me some buffer lines and a width, I give you back a pile of Cells.

That "dumb and single-purpose" quality is deliberate. Every render can be written, tested, and modified in isolation. Adding Mermaid support means creating `render_mermaid.go`, adding one detection branch in the detector, and never touching any other render.

Take `RenderTable`, the most dramatic one. Give it these three buffer lines:

```
| name | age |
|------|-----|
| ada  | 36  |
```

It scans all three to compute column widths, and outputs a 5-row `RenderedSegment`:

```
Row 0: ┌──────┬─────┐    ← BufLine = -1 (decorative, drawn from nothing)
Row 1: │ name │ age │    ← BufLine = 0 (maps to buffer line 0)
Row 2: ├──────┼─────┤    ← BufLine = -1
Row 3: │ ada  │ 36  │    ← BufLine = 2 (buffer line 1 is the separator; it's eaten)
Row 4: └──────┴─────┘    ← BufLine = -1
```

Three things, each impossible for a line editor:

1. **5 screen lines come from 3 buffer lines.** The extra borders don't exist in the buffer.
2. **Buffer line 1 (`|---|---|`) vanishes entirely from the screen.** It only tells the renderer "this is the header separator." Once rendered, it's consumed.
3. **Every Cell knows which buffer position it maps to.** The user clicks the `a` in `ada` on screen row 3; the system finds `BufLine=2, BufX=2`, and the cursor lands precisely on the `a` in `| ada |` in the buffer. Click a border, `IsDecorative=true`, the click is ignored or mapped to the nearest real line.

---

### 5. The pipeline connects: from buffer to screen

With Segment (§3) and renders (§4) in place, the last thing is to wire them together. The full pipeline looks like this:

```
buffer change
    ↓
DetectSegments()              ← central classifier, scans the whole buffer
    ↓
[]Segment                     ← each buffer line belongs to one Segment, with a Render pointer
    ↓
displayBufferMD()             ← render entry point, called every frame
    ↓
iterate visible Segments → call seg.Render(seg, width, cfg)
    ↓
[]RenderedSegment (all Cells)
    ↓
Cells written to the tcell screen
```

There's one key design here, which I mentioned in §3: **detect and render are two fully independent stages.**

- `DetectSegments()` runs the moment the buffer changes. It scans line by line, using each line's character signature — `#` at the start means heading, `|` means table, consecutive `>` means blockquote… — to slice the whole buffer into a list of Segments. The entire classifier is under 80 lines.
- Detect **knows nothing about screen width, knows nothing about scrolling, knows nothing about the cursor.** It only depends on buffer content. This guarantees the same buffer produces the same classification regardless of terminal size.
- Render is the one that cares about width. That `width` in `RenderTable(seg, width, ...)` is the current terminal's column count — tables compute their column widths from it.

Why split it this cleanly? Because detect's result is consumed by two different places: rendering needs it, and **so does scrolling.** Micro's scroll system needs to know "how many screen rows does this segment occupy" in order to compute where PageUp lands — and that's screen-width-independent, but buffer-content-dependent. So detect has to exist independently of render, available to both. (This came back to bite me later. A story for next time.)

---

At this point the foundational architecture was complete:

- buffer untouched, tcell untouched, a Segment layer wedged between
- every buffer line belongs to a Segment, each holding a render
- detect classifies, render translates, display writes to screen
- every Cell carries its buffer coordinates, so clicks, selections, and cursor all reverse-map correctly

**Rendering was finally done.** I'd open a `.md` file and see nicely typeset headings, tables, and code blocks — not the raw, crude formatting characters.

I gave the new baby a name: "microNeo".

---

microNeo is open source: [github.com/sollawen/microNeo](https://github.com/sollawen/microNeo). One-line install, works great as `$EDITOR` for Claude Code, Yazi, and friends.

---

## Translation notes (internal, not for publishing)

### What I deliberately preserved from your Chinese voice
- **Phenomenon-first, conclusion-last.** The opening doesn't say "I wanted to build X" — it shows the era (vibe coding) shifting your habits, then the need surfaces. English preserves this exactly: "But then vibe coding arrived, and I found myself writing less and less code."
- **The era is the first subject.** "vibe coding arrived" — the era moves first, the author is moved by it. Not "I decided to use AI."
- **Short sentences, real beats.** "I tolerated it for a few months. Then I couldn't." mirrors your "忍了几个月之后，我忍不了了。" Two beats, then the action. I refused to inflate this into "After several months of frustration with existing tools, I finally decided to take action."
- **The 1:1 term framing.** "line editor" opens, "rich-format editor" closes — your term rhyme is preserved.

### Vocabulary calls
- **vibe coding**: kept verbatim. It's a real HN/dev.to term (Karpathy-coined), carries era + trigger + AI-era market positioning all at once. Better than "AI-assisted coding" which is bloodless.
- **independent fork**: your correction from yesterday. Not "a fork," not "a branch." microNeo is its own product that will never merge back.
- **rich-format editor**: literal translation of "rich format editor." Tested alternatives — "WYSIWYG editor," "render editor" (used in title), "structured editor" — none felt as direct. "rich-format" keeps the contrast with "line" editor sharp.
- **segment / render / Cell**: kept as code identifiers verbatim. They're real Go types, not translatable concepts.
- **"drawn out of thin air"**: for "凭空画出来." This is one of your quotable lines per the §3 self-check; English needs it to land.
- **"came back to bite me"**: for "捅出一个大篓子." More colloquial than "caused a serious problem later," matches your casual Chinese voice.

### Things I did NOT translate literally
- "傻傻的字符" → "silly pipe characters." "Silly characters" alone is ambiguous in English; "silly pipe characters" keeps your tone while being precise.
- "我给这个新的micro，起了个名字" → "I gave the new baby a name." 用户亲自改的版本，抓住了原文的人格化/亲昵语气（"新的 micro" = 新生儿，不是 "new version of Micro" 的 PM 腔）。
- "（这件事后来捅出一个大篓子。下一篇讲。）" → "(This came back to bite me later. A story for next time.)" Kept the parenthetical aside — it's your voice, casual and forward-pointing. The reader gets a hint there's more without it being a series announcement.

### Word count
~1850 words. In target range (1600-2000).

### Open questions for you
1. **"vibe coding"** — kept verbatim. If you prefer something more established for an HN audience, alternatives: "AI-pair programming," "the age of LLM-written Markdown." My vote: keep "vibe coding."
2. **"rich-format editor"** (opening) vs "render editor" (rejected title candidates) — kept both. Opening uses "rich-format" to contrast with "line"; no title anymore so the tension is resolved.
3. **Dev.to cover image** — should be a real microNeo screenshot of a rendered table, not the ASCII art. Worth grabbing before publishing.
