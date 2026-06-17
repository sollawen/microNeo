**v1.0.6**
- Rewrite the MD rendering pipeline around a single `screenBuffer` data source, replacing the v1.0.5 dual-write viewportRowmap. Eliminates cross-segment cursor drift caused by screen/rowmap timing inconsistency.
- Split the render pipeline into four single-responsibility stages: `DetectSegments` (buffer-driven, width-independent) → `displayToBuffer` (writes to sb once) → `showBuffer` (pure blit) → `relocateVerticalMD` (pure query).
- Export `LineToScreenRow` / `ScreenRowToLine` for screen↔buffer coordinate conversion.
- Remove the `MDRender` and `MDRenderIdle` config options (rendering is now unconditional when MD is enabled).
- Improve table rendering and add display-package unit tests.
- Fix: cursor no longer drifts or disappears when navigating across table/code-block segments (continuous ↓, goto, search).
- Fix: scrollup no longer hides the cursor under the status line or leaves blank regions after the viewport.
- Fix: mouse scroll no longer gets stuck or falls back to native formatting mid-scroll.
- Fix: continuous ↓ across a table no longer makes the viewport jump — case A/C now judges against the visible viewport `[startVY, startVY+height]`, accounting for the blit offset after scrollup.
- Fix: clicking a table/code-block decoration row no longer jumps the cursor far away and scrolls the viewport — decoration rows now map down to the nearest content row (or the last line at file end), and the case boundary is left-closed-right-closed so "cursor exactly 1 row below the visible bottom" takes the small-scrollup path (case A) instead of the big-jump path (case C).
- Fix: prevent MD render overflow fallback when the cursor leaves the viewport; use `nContent` for viewport invalidation check.

**v1.0.5**
- Move MD cursor vertical scroll relocation from buffer-line space to screen-line space: add a 2D viewportRowmap that matches the cursor's screen position by (Line, Row).
- Dispatch vertical Relocate: MD files go through `relocateVerticalMD`; non-MD files keep micro's native logic (zero intrusion).
- Fix end-of-file panic: `renderSegmentNative` could overflow `vY` on the last line's softwrap, causing a `viewportRowBufLine` slice out-of-bounds; now truncated to bufHeight.
- `render_table.makeTableSeparator` takes a bufLine argument so code-block header separators map to the real buffer line.
- Tweak s-dark colorscheme `statusline.special/dim/normal` colors.
- Remove the debug log written to `/tmp/microNeo-debug.log` from `initMDConfig`.

**v1.0.4**
- `viewportRowBufLine` flat array replaces `mdCache` for O(1) screen row → buffer line lookup.
- Add `bufferLineToScreenOffset` reverse lookup.
- Code-block top/bottom borders now map to real buffer lines (opening/closing fence).

**v1.0.3**
- Inline-code background color now renders correctly in all renderers.
- Tweak s-light colors.
