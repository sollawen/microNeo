# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.7] - 2026-06-17

### Changed
- Align the MD diagnostic log with micro's native `util.Debug` switch. `dbgLog` now writes `/tmp/microNeo_debug.log` only when `util.Debug == "ON"` (i.e. `make build-dbg`). Release builds via `make build` / `make build-quick` default to `OFF`, so the log is fully disabled with near-zero overhead. Previously the switch was a hardcoded `const microNeoDebug = true`, so every build unconditionally appended to the log.

### Fixed
- Pressing Enter to add a new line at the end of the buffer no longer triggers an erroneous screen relocate. After Enter, the cursor lands on the newly appended buffer line, but the previous frame's `screenBuffer` had not rendered that line yet, so `rowIndexOf(c)` returned `(0, false)`. This caused `relocateVerticalMD`'s caseJudge to misfire as case C, jumping `StartLine` to `c.Line - scrollmargin` and collapsing the screen to only the last 4 lines. Fix: when `rowIndexOf(c)` fails but `c.Line == sb.lastLine+1` (the typical Enter / continuous ↓ case), estimate `curRow = lastRow + 1` and keep judging against the visible viewport via case A, so continuous ↓ / Enter now produces a smooth scrollup of `delta=1`.

## [1.0.6] - 2026-06-17

### Added
- Export `LineToScreenRow` / `ScreenRowToLine` for screen↔buffer coordinate conversion.

### Changed
- Rewrite the MD rendering pipeline around a single `screenBuffer` data source, replacing the v1.0.5 dual-write viewportRowmap. Eliminates cross-segment cursor drift caused by screen/rowmap timing inconsistency.
- Split the render pipeline into four single-responsibility stages: `DetectSegments` (buffer-driven, width-independent) → `displayToBuffer` (writes to sb once) → `showBuffer` (pure blit) → `relocateVerticalMD` (pure query).
- Improve table rendering and add display-package unit tests.

### Fixed
- Cursor no longer drifts or disappears when navigating across table/code-block segments (continuous ↓, goto, search).
- Scrollup no longer hides the cursor under the status line or leaves blank regions after the viewport.
- Mouse scroll no longer gets stuck or falls back to native formatting mid-scroll.
- Continuous ↓ across a table no longer makes the viewport jump — case A/C now judges against the visible viewport `[startVY, startVY+height]`, accounting for the blit offset after scrollup.
- Clicking a table/code-block decoration row no longer jumps the cursor far away and scrolls the viewport — decoration rows now map down to the nearest content row (or the last line at file end), and the case boundary is left-closed-right-closed so "cursor exactly 1 row below the visible bottom" takes the small-scrollup path (case A) instead of the big-jump path (case C).
- Prevented MD render overflow fallback when the cursor leaves the viewport; use `nContent` for viewport invalidation check.

### Removed
- The `MDRender` and `MDRenderIdle` config options (rendering is now unconditional when MD is enabled).

## [1.0.5] - 2026-06-15

### Changed
- Move MD cursor vertical scroll relocation from buffer-line space to screen-line space: add a 2D viewportRowmap that matches the cursor's screen position by (Line, Row).
- Dispatch vertical Relocate: MD files go through `relocateVerticalMD`; non-MD files keep micro's native logic (zero intrusion).
- `render_table.makeTableSeparator` now takes a bufLine argument so code-block header separators map to the real buffer line.
- Tweak s-dark colorscheme `statusline.special/dim/normal` colors.

### Fixed
- End-of-file panic: `renderSegmentNative` could overflow `vY` on the last line's softwrap, causing a `viewportRowBufLine` slice out-of-bounds; now truncated to bufHeight.

### Removed
- Debug log written to `/tmp/microNeo-debug.log` from `initMDConfig`.

## [1.0.4] - 2026-06-11

### Added
- Add `bufferLineToScreenOffset` reverse lookup.

### Changed
- `viewportRowBufLine` flat array replaces `mdCache` for O(1) screen row → buffer line lookup.

### Fixed
- Code-block top/bottom borders now map to real buffer lines (opening/closing fence).

## [1.0.3] - 2026-06-09

### Changed
- Tweak s-light colors.

### Fixed
- Inline-code background color now renders correctly in all renderers.
