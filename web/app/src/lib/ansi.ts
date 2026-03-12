/**
 * ANSI escape code parser for terminal output.
 *
 * Converts a raw PTY byte stream into styled HTML spans. Handles:
 * - SGR (Select Graphic Rendition) — colors, bold, italic, underline, etc.
 * - 256-color and truecolor (24-bit) sequences
 * - Carriage return (\r) — moves cursor to column 0 (for progress bars, prompt redraw)
 * - Newline (\n), backspace (\b), tab (\t)
 * - CSI K (erase in line), CSI J (erase in display)
 * - CSI G (cursor horizontal absolute) — used by bash for prompt redrawing
 * - CSI C/D (cursor forward/backward) — used by readline for cursor movement
 * - Strips OSC, character set designations, and other unhandled sequences
 */

// Standard 4-bit ANSI colors (Tokyo Night palette)
const COLORS_16 = [
  '#15161e', '#f7768e', '#9ece6a', '#e0af68', '#7aa2f7', '#bb9af7', '#7dcfff', '#a9b1d6', // 0-7
  '#414868', '#f7768e', '#9ece6a', '#e0af68', '#7aa2f7', '#bb9af7', '#7dcfff', '#c0caf5', // 8-15 (bright)
];

/** Convert 256-color index to hex */
function color256(n: number): string {
  if (n < 16) return COLORS_16[n];
  if (n >= 232) { const g = 8 + (n - 232) * 10; return `rgb(${g},${g},${g})`; }
  const i = n - 16;
  const r = Math.floor(i / 36) * 51;
  const g = (Math.floor(i / 6) % 6) * 51;
  const b = (i % 6) * 51;
  return `rgb(${r},${g},${b})`;
}

export interface AnsiStyle {
  fg: string | null;
  bg: string | null;
  bold: boolean;
  dim: boolean;
  italic: boolean;
  underline: boolean;
  strikethrough: boolean;
  inverse: boolean;
}

const DEFAULT_STYLE: Readonly<AnsiStyle> = Object.freeze({
  fg: null, bg: null, bold: false, dim: false,
  italic: false, underline: false, strikethrough: false, inverse: false,
});

function defaultStyle(): AnsiStyle {
  return { ...DEFAULT_STYLE };
}

/** Check if two styles are identical (used for span merging) */
function stylesEqual(a: AnsiStyle, b: AnsiStyle): boolean {
  return a.fg === b.fg && a.bg === b.bg &&
    a.bold === b.bold && a.dim === b.dim &&
    a.italic === b.italic && a.underline === b.underline &&
    a.strikethrough === b.strikethrough && a.inverse === b.inverse;
}

/** A styled text span */
export interface StyledSpan {
  text: string;
  style: AnsiStyle;
}

/** A terminal line is an array of styled spans */
export type TermLine = StyledSpan[];

/**
 * Flatten a TermLine into a single string (text content only).
 * Useful for computing column positions during cursor movement.
 */
function lineToString(line: StyledSpan[]): string {
  let s = '';
  for (const span of line) s += span.text;
  return s;
}

/**
 * Rebuild a TermLine by overwriting characters starting at `col` with `text` styled by `style`.
 * Characters before `col` keep their existing styles; characters after the overwritten region
 * are preserved. If `col` is past the end, spaces (default style) fill the gap.
 */
function overwriteLineAt(line: StyledSpan[], col: number, text: string, style: AnsiStyle): StyledSpan[] {
  if (text.length === 0) return line;

  const oldStr = lineToString(line);
  const endCol = col + text.length;

  // Build a per-character style map from existing spans
  const charStyles: AnsiStyle[] = [];
  for (const span of line) {
    for (let j = 0; j < span.text.length; j++) {
      charStyles.push(span.style);
    }
  }

  // Build the new string
  let newStr = '';
  // Part 1: existing chars before col (pad with spaces if needed)
  if (col <= oldStr.length) {
    newStr = oldStr.slice(0, col);
  } else {
    newStr = oldStr + ' '.repeat(col - oldStr.length);
    // Fill charStyles for the gap
    const ds = defaultStyle();
    while (charStyles.length < col) charStyles.push(ds);
  }
  // Part 2: overwritten text
  newStr += text;
  // Part 3: remaining old text after the overwritten region
  if (endCol < oldStr.length) {
    newStr += oldStr.slice(endCol);
  }

  // Assign styles for the overwritten region
  while (charStyles.length < newStr.length) charStyles.push(defaultStyle());
  for (let j = col; j < col + text.length; j++) {
    charStyles[j] = style;
  }

  // Rebuild spans by merging consecutive chars with the same style
  const result: StyledSpan[] = [];
  let i = 0;
  while (i < newStr.length) {
    const s = charStyles[i];
    let end = i + 1;
    while (end < newStr.length && stylesEqual(charStyles[end], s)) end++;
    result.push({ text: newStr.slice(i, end), style: s });
    i = end;
  }
  return result;
}

/**
 * Truncate a TermLine at a given column (erase from col to end).
 */
function truncateLineAt(line: StyledSpan[], col: number): StyledSpan[] {
  if (col === 0) return [];
  const result: StyledSpan[] = [];
  let pos = 0;
  for (const span of line) {
    if (pos >= col) break;
    if (pos + span.text.length <= col) {
      result.push(span);
    } else {
      result.push({ text: span.text.slice(0, col - pos), style: span.style });
    }
    pos += span.text.length;
  }
  return result;
}

/**
 * AnsiParser maintains terminal state and produces lines of styled text.
 * Tracks cursor column position for proper handling of \r, CSI G, CSI C/D, CSI K.
 */
export class AnsiParser {
  private style: AnsiStyle = defaultStyle();
  private currentLine: StyledSpan[] = [];
  private currentText = '';
  private cursorCol = 0;
  private lines: TermLine[] = [];

  // Parser state machine
  private state: 'normal' | 'escape' | 'csi' | 'osc' = 'normal';
  private csiParams = '';
  private oscData = '';

  /** Max lines to keep in buffer */
  private maxLines = 5000;

  /** Get all lines */
  getLines(): TermLine[] {
    return this.lines;
  }

  /** Get current (incomplete) line */
  getCurrentLine(): TermLine {
    if (this.currentText) {
      // Merge currentText into the line at cursorCol
      const merged = overwriteLineAt(
        this.currentLine,
        this.cursorCol - this.currentText.length,
        this.currentText,
        this.style,
      );
      return merged;
    }
    return this.currentLine;
  }

  /** Clear all output */
  clear(): void {
    this.lines = [];
    this.currentLine = [];
    this.currentText = '';
    this.cursorCol = 0;
  }

  /** Feed raw text data into the parser */
  feed(data: string): void {
    for (let i = 0; i < data.length; i++) {
      const code = data.charCodeAt(i);

      // Handle surrogate pairs (emoji and other characters above U+FFFF)
      let ch: string;
      if (code >= 0xd800 && code <= 0xdbff && i + 1 < data.length) {
        const lo = data.charCodeAt(i + 1);
        if (lo >= 0xdc00 && lo <= 0xdfff) {
          ch = data[i] + data[i + 1];
          i++; // skip the low surrogate
        } else {
          ch = data[i];
        }
      } else {
        ch = data[i];
      }

      switch (this.state) {
        case 'normal':
          if (code === 0x1b) { // ESC
            this.state = 'escape';
          } else if (ch === '\n') {
            this.flushText();
            this.pushLine();
            this.cursorCol = 0;
          } else if (ch === '\r') {
            // Carriage return — move cursor to column 0 (don't clear content;
            // subsequent text will overwrite via column tracking)
            this.flushText();
            this.cursorCol = 0;
          } else if (ch === '\b') {
            // Backspace — move cursor left one column
            this.flushText();
            if (this.cursorCol > 0) this.cursorCol--;
          } else if (ch === '\t') {
            // Tab — expand to spaces (8-column stops)
            this.flushText();
            const spaces = 8 - (this.cursorCol % 8);
            this.currentText = ' '.repeat(spaces);
            this.cursorCol += spaces;
          } else if (code >= 0x20 || ch.length > 1) {
            // Printable character (or multi-byte codepoint)
            this.currentText += ch;
            this.cursorCol++;
          }
          // Ignore other control characters (BEL, etc.)
          break;

        case 'escape':
          if (ch === '[') {
            this.state = 'csi';
            this.csiParams = '';
          } else if (ch === ']') {
            this.state = 'osc';
            this.oscData = '';
          } else if (ch === '(' || ch === ')') {
            // Character set designation — skip next char
            i++;
            this.state = 'normal';
          } else if (ch === '>' || ch === '=' || ch === '#') {
            // Keypad mode, DECALN — skip
            this.state = 'normal';
          } else if (ch === 'M') {
            // Reverse index (RI) — scroll down; treat as newline for simplicity
            this.state = 'normal';
          } else if (ch === '7' || ch === '8') {
            // Save/restore cursor (DECSC/DECRC) — ignore
            this.state = 'normal';
          } else {
            // Unknown escape — ignore and return to normal
            this.state = 'normal';
          }
          break;

        case 'csi':
          if ((code >= 0x30 && code <= 0x3f)) {
            // Parameter bytes: 0-9, ;, <, =, >, ?
            this.csiParams += ch;
          } else if (code >= 0x20 && code <= 0x2f) {
            // Intermediate bytes — add to params
            this.csiParams += ch;
          } else if (code >= 0x40 && code <= 0x7e) {
            // Final byte — execute CSI sequence
            this.handleCSI(ch, this.csiParams);
            this.state = 'normal';
          } else {
            // Invalid — bail
            this.state = 'normal';
          }
          break;

        case 'osc':
          // OSC sequences end with BEL (\x07) or ST (ESC \)
          if (code === 0x07) {
            this.state = 'normal';
          } else if (code === 0x1b) {
            // Might be ST (ESC \) — peek ahead
            if (i + 1 < data.length && data[i + 1] === '\\') {
              i++;
              this.state = 'normal';
            }
          } else {
            this.oscData += ch;
          }
          break;
      }
    }
  }

  private handleCSI(finalByte: string, params: string): void {
    // Skip private-mode sequences (params starting with ? or >)
    if (params.length > 0 && (params[0] === '?' || params[0] === '>')) {
      // Private mode set/reset (DECSET/DECRST, e.g. ?25h cursor visibility,
      // ?1049h alternate screen) — silently ignore
      return;
    }

    const firstParam = parseInt(params) || 0;

    switch (finalByte) {
      case 'm': // SGR — Select Graphic Rendition
        this.flushText();
        this.handleSGR(params);
        break;

      case 'G': { // CHA — Cursor Horizontal Absolute (1-based)
        this.flushText();
        const col = (parseInt(params) || 1) - 1; // convert to 0-based
        this.cursorCol = Math.max(0, col);
        break;
      }

      case 'C': { // CUF — Cursor Forward
        this.flushText();
        const n = parseInt(params) || 1;
        this.cursorCol += n;
        break;
      }

      case 'D': { // CUB — Cursor Backward
        this.flushText();
        const n = parseInt(params) || 1;
        this.cursorCol = Math.max(0, this.cursorCol - n);
        break;
      }

      case 'A': { // CUU — Cursor Up (ignored in our line-buffer model)
        break;
      }

      case 'B': { // CUD — Cursor Down (ignored in our line-buffer model)
        break;
      }

      case 'H':   // CUP — Cursor Position (row;col)
      case 'f': { // HVP — same as CUP
        // We only handle the column component; row movement doesn't map well
        // to our append-only line buffer.
        this.flushText();
        const parts = params.split(';');
        const col = (parseInt(parts[1]) || 1) - 1;
        this.cursorCol = Math.max(0, col);
        break;
      }

      case 'J': { // ED — Erase in Display
        this.flushText();
        const n = firstParam;
        if (n === 0) {
          // Erase below — clear from cursor to end of screen
          // Truncate current line at cursor, remove all lines after
          this.currentLine = truncateLineAt(this.currentLine, this.cursorCol);
        } else if (n === 2 || n === 3) {
          // Clear entire screen
          this.lines = [];
          this.currentLine = [];
          this.currentText = '';
          this.cursorCol = 0;
        }
        break;
      }

      case 'K': { // EL — Erase in Line
        this.flushText();
        const n = firstParam;
        if (n === 0) {
          // Erase from cursor to end of line
          this.currentLine = truncateLineAt(this.currentLine, this.cursorCol);
        } else if (n === 1) {
          // Erase from start of line to cursor (replace with spaces)
          const fullStr = lineToString(this.currentLine);
          if (this.cursorCol > 0) {
            const spaces = ' '.repeat(Math.min(this.cursorCol, fullStr.length));
            this.currentLine = overwriteLineAt(this.currentLine, 0, spaces, defaultStyle());
          }
        } else if (n === 2) {
          // Erase entire line
          this.currentLine = [];
          this.cursorCol = 0;
        }
        break;
      }

      case 'X': { // ECH — Erase Characters (replace N chars at cursor with spaces)
        this.flushText();
        const n = parseInt(params) || 1;
        const spaces = ' '.repeat(n);
        this.currentLine = overwriteLineAt(this.currentLine, this.cursorCol, spaces, defaultStyle());
        break;
      }

      case 'P': { // DCH — Delete Characters (shift left)
        this.flushText();
        const n = parseInt(params) || 1;
        const str = lineToString(this.currentLine);
        if (this.cursorCol < str.length) {
          // Truncate at cursor, then append remaining content after deleted chars
          this.currentLine = truncateLineAt(this.currentLine, this.cursorCol);
          if (this.cursorCol + n < str.length) {
            const remaining = str.slice(this.cursorCol + n);
            this.currentLine = overwriteLineAt(this.currentLine, this.cursorCol, remaining, this.style);
          }
        }
        break;
      }

      case '@': { // ICH — Insert Characters (shift right, insert spaces)
        this.flushText();
        const n = parseInt(params) || 1;
        const str = lineToString(this.currentLine);
        if (this.cursorCol <= str.length) {
          // Insert N spaces at cursor, pushing existing content right
          this.currentLine = overwriteLineAt(this.currentLine, this.cursorCol, ' '.repeat(n) + str.slice(this.cursorCol), this.style);
        }
        break;
      }

      case 'h': // SM — Set Mode (ignored)
      case 'l': // RM — Reset Mode (ignored)
      case 'r': // DECSTBM — Set Scrolling Region (ignored)
      case 'n': // DSR — Device Status Report (ignored)
      case 's': // SCP — Save Cursor Position (ignored)
      case 'u': // RCP — Restore Cursor Position (ignored)
      case 'c': // DA — Device Attributes (ignored)
      case 't': // Window manipulation (ignored)
      case 'S': // SU — Scroll Up (ignored)
      case 'T': // SD — Scroll Down (ignored)
      case 'L': // IL — Insert Lines (ignored)
      case 'M': // DL — Delete Lines (ignored)
        break;

      // All other CSI sequences — silently ignored
    }
  }

  private handleSGR(params: string): void {
    const parts = params === '' ? [0] : params.split(';').map(n => parseInt(n) || 0);

    for (let i = 0; i < parts.length; i++) {
      const p = parts[i];

      if (p === 0) { this.style = defaultStyle(); }
      else if (p === 1) this.style.bold = true;
      else if (p === 2) this.style.dim = true;
      else if (p === 3) this.style.italic = true;
      else if (p === 4) this.style.underline = true;
      else if (p === 7) this.style.inverse = true;
      else if (p === 8) { /* hidden — not supported, skip */ }
      else if (p === 9) this.style.strikethrough = true;
      else if (p === 21) this.style.underline = true; // double underline (treat as underline)
      else if (p === 22) { this.style.bold = false; this.style.dim = false; }
      else if (p === 23) this.style.italic = false;
      else if (p === 24) this.style.underline = false;
      else if (p === 25) { /* blink off — we don't support blink */ }
      else if (p === 27) this.style.inverse = false;
      else if (p === 28) { /* reveal (unhide) — skip */ }
      else if (p === 29) this.style.strikethrough = false;
      else if (p >= 30 && p <= 37) this.style.fg = COLORS_16[p - 30];
      else if (p === 38) {
        if (parts[i + 1] === 5 && i + 2 < parts.length) {
          this.style.fg = color256(parts[i + 2] || 0); i += 2;
        } else if (parts[i + 1] === 2 && i + 4 < parts.length) {
          this.style.fg = `rgb(${parts[i+2]||0},${parts[i+3]||0},${parts[i+4]||0})`; i += 4;
        }
      }
      else if (p === 39) this.style.fg = null;
      else if (p >= 40 && p <= 47) this.style.bg = COLORS_16[p - 40];
      else if (p === 48) {
        if (parts[i + 1] === 5 && i + 2 < parts.length) {
          this.style.bg = color256(parts[i + 2] || 0); i += 2;
        } else if (parts[i + 1] === 2 && i + 4 < parts.length) {
          this.style.bg = `rgb(${parts[i+2]||0},${parts[i+3]||0},${parts[i+4]||0})`; i += 4;
        }
      }
      else if (p === 49) this.style.bg = null;
      else if (p >= 90 && p <= 97) this.style.fg = COLORS_16[p - 90 + 8];
      else if (p >= 100 && p <= 107) this.style.bg = COLORS_16[p - 100 + 8];
    }
  }

  /** Flush currentText into the line at the correct cursor position */
  private flushText(): void {
    if (!this.currentText) return;

    const writeCol = this.cursorCol - this.currentText.length;
    const lineLen = this.lineContentLength();

    if (writeCol >= lineLen) {
      // Appending at or past the end — fast path
      if (writeCol > lineLen) {
        // Gap between existing content and cursor — fill with spaces
        this.currentLine.push({ text: ' '.repeat(writeCol - lineLen), style: defaultStyle() });
      }
      // Try to merge with last span if styles match
      const last = this.currentLine.length > 0 ? this.currentLine[this.currentLine.length - 1] : null;
      if (last && stylesEqual(last.style, this.style)) {
        last.text += this.currentText;
      } else {
        this.currentLine.push({ text: this.currentText, style: { ...this.style } });
      }
    } else {
      // Overwriting existing content — use the overwrite helper
      this.currentLine = overwriteLineAt(this.currentLine, writeCol, this.currentText, { ...this.style });
    }
    this.currentText = '';
  }

  private pushLine(): void {
    this.lines.push(this.currentLine);
    this.currentLine = [];
    // Trim to max lines — use shift-based trimming when only slightly over
    if (this.lines.length > this.maxLines + 500) {
      this.lines = this.lines.slice(-this.maxLines);
    }
  }

  /** Length of committed spans in currentLine (not including currentText) */
  private lineContentLength(): number {
    let len = 0;
    for (const span of this.currentLine) len += span.text.length;
    return len;
  }
}

/** Convert a styled span to an inline style string */
export function spanToStyle(style: AnsiStyle): string | undefined {
  const parts: string[] = [];
  let fg = style.fg;
  let bg = style.bg;

  if (style.inverse) { [fg, bg] = [bg || 'var(--sv-terminal-bg)', fg || 'var(--sv-terminal-fg)']; }
  if (fg) parts.push(`color:${fg}`);
  if (bg) parts.push(`background:${bg}`);
  if (style.bold) parts.push('font-weight:bold');
  if (style.dim) parts.push('opacity:0.6');
  if (style.italic) parts.push('font-style:italic');

  // Combine text-decoration values (underline + line-through can coexist)
  const decorations: string[] = [];
  if (style.underline) decorations.push('underline');
  if (style.strikethrough) decorations.push('line-through');
  if (decorations.length > 0) parts.push(`text-decoration:${decorations.join(' ')}`);

  return parts.length > 0 ? parts.join(';') : undefined;
}
