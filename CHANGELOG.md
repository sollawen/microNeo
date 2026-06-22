# Changelog

All notable changes to this project will be documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [1.0.11] - 2026-06-22

### Fixed
- Pasting a large block of text into an empty Markdown file no longer shows only the last `scrollmargin + 1` rows of the buffer. The cursor's vertical relocation in MD files was missing the `end-pin` branch (micro native Relocate branch 4): when the cursor lands near `bEnd` (short buffer / paste / goto-end), the view start was over-pushed toward the end and `coversExtent`'s buffer-end exception then locked the wrong state in place. `relocateVerticalMD` case C now branches on cursor position relative to `bEnd` — middle region keeps the old estimate, end region pins `bEnd` to the viewport bottom — restoring one-to-one alignment with micro's native 4-branch Relocate.

## [1.0.10] - 2026-06-19

### Fixed
- Multi-cursor display regression in Markdown files: pressing `Shift-Alt-Up/Down` now shows all cursors again, instead of only the last one. Input/delete already worked correctly — only the on-screen rendering of secondary cursors was lost. Introduced in v1.0.6 (screenBuffer refactor).

## [1.0.9] - 2026-06-19

### Fixed
- Typing at end of last line that triggers a softwrap no longer yanks the cursor to the top of the viewport.

## [1.0.8] - 2026-06-17

### Fixed
- Pressing ESC in a Markdown file no longer collapses the whole screen back to raw markdown formatting.

## [1.0.7] - 2026-06-17

### Changed
- MD diagnostic log now follows micro's debug switch, off by default in release builds.

### Fixed
- Pressing Enter at the end of a buffer no longer causes incorrect screen scrolling.

## [1.0.6] - 2026-06-17

### Added
- Export screen↔buffer coordinate conversion functions.

### Changed
- Rewrite MD rendering pipeline to eliminate cursor drift when navigating across table and code-block segments.
- Improve table rendering and add display-package unit tests.

### Fixed
- Cursor no longer drifts or disappears when navigating across table/code-block segments.
- Scrolling up no longer hides the cursor or leaves blank regions.
- Mouse scroll no longer gets stuck or falls back to raw formatting mid-scroll.
- Continuous ↓ across a table no longer makes the viewport jump.
- Clicking a table or code-block decoration row no longer jumps the cursor unexpectedly.

### Removed
- `MDRender` and `MDRenderIdle` config options (rendering is now unconditional when MD is enabled).

## [1.0.5] - 2026-06-15

### Changed
- Cursor vertical scrolling now works correctly in MD files, including across table and code-block segments.
- Tweak s-dark colorscheme status line colors.

### Fixed
- End-of-file panic when softwrapping at the last line.

### Removed
- Debug log from `initMDConfig`.

## [1.0.4] - 2026-06-11

### Added
- Add screen offset reverse lookup function.

### Changed
- Screen row → buffer line lookup now uses O(1) flat array.

### Fixed
- Code-block borders now map to real buffer lines.

## [1.0.3] - 2026-06-09

### Changed
- Tweak s-light colors.

### Fixed
- Inline-code background color now renders correctly in all renderers.
