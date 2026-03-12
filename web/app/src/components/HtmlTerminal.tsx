import { useEffect, useRef, useCallback, useState } from 'react';
import { AnsiParser, spanToStyle, type TermLine } from '../lib/ansi';
import { useWebSocket, type ConnectionState } from '../hooks/useWebSocket';
import { detectOptions, QUICK_REPLY_EVENT, type QuickReplyEventDetail } from './QuickReply';
import styles from './HtmlTerminal.module.css';

/** URL pattern for Claude OAuth */
const OAUTH_URL_RE = /https:\/\/claude\.ai\/oauth\/authorize[^\s\x00-\x1f]*/;

export type TerminalActivity = 'idle' | 'streaming' | 'waiting';

interface HtmlTerminalProps {
  onConnectionChange: (state: ConnectionState) => void;
  onDisconnectedAtChange?: (ts: number | null) => void;
  onForceReconnectReady?: (fn: () => void) => void;
  onHapticConnect?: () => void;
  onHapticDisconnect?: () => void;
  onActivityChange?: (activity: TerminalActivity) => void;
}

/**
 * HtmlTerminal — native HTML terminal rendering.
 *
 * Replaces xterm.js with plain HTML divs + ANSI color parsing.
 * Benefits: native scroll, native text selection, native copy, works on mobile.
 * Tradeoff: no cursor positioning or alternate screen (Claude Code TUI degrades to linear text).
 */
export default function HtmlTerminal({ onConnectionChange, onDisconnectedAtChange, onForceReconnectReady, onHapticConnect, onHapticDisconnect, onActivityChange }: HtmlTerminalProps) {
  const scrollRef = useRef<HTMLDivElement>(null);
  const parserRef = useRef(new AnsiParser());
  const sendRef = useRef<(data: string | Uint8Array) => void>(() => {});
  const [, setRenderTick] = useState(0);
  const autoScrollRef = useRef(true);
  const newLinesSinceScrollRef = useRef(0);
  const [showNewOutputPill, setShowNewOutputPill] = useState(false);
  const urlBufferRef = useRef('');
  const quickReplyTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const lastOptionsJsonRef = useRef<string>('');

  // Activity tracking refs
  const activityRef = useRef<TerminalActivity>('idle');
  const lastDataTimeRef = useRef<number>(0);
  const userSentTimeRef = useRef<number>(0);
  const activityTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const onActivityChangeRef = useRef(onActivityChange);
  onActivityChangeRef.current = onActivityChange;

  const setActivity = useCallback((next: TerminalActivity) => {
    if (activityRef.current !== next) {
      activityRef.current = next;
      onActivityChangeRef.current?.(next);
    }
  }, []);

  // Track whether user has scrolled up (disable auto-scroll)
  const handleScroll = useCallback(() => {
    const el = scrollRef.current;
    if (!el) return;
    const atBottom = el.scrollHeight - el.scrollTop - el.clientHeight < 50;
    autoScrollRef.current = atBottom;
    if (atBottom) {
      newLinesSinceScrollRef.current = 0;
      setShowNewOutputPill(false);
    }
  }, []);

  // Terminal write callback for WebSocket
  const handleData = useCallback((data: ArrayBuffer) => {
    const text = new TextDecoder().decode(data);
    parserRef.current.feed(text);
    setRenderTick(t => t + 1);

    // Track new output while user is scrolled up
    if (!autoScrollRef.current) {
      // Count newlines in the incoming chunk as a proxy for new lines
      const newlineCount = (text.match(/\n/g) || []).length || 1;
      newLinesSinceScrollRef.current += newlineCount;
      setShowNewOutputPill(true);
    }

    // Activity tracking: data received -> streaming
    lastDataTimeRef.current = Date.now();
    setActivity('streaming');

    // After 3s of no data, transition to idle
    if (activityTimerRef.current) clearTimeout(activityTimerRef.current);
    activityTimerRef.current = setTimeout(() => {
      setActivity('idle');
    }, 3000);

    // Auto-scroll to bottom
    requestAnimationFrame(() => {
      if (autoScrollRef.current && scrollRef.current) {
        scrollRef.current.scrollTop = scrollRef.current.scrollHeight;
      }
    });

    // Scan for OAuth URLs using a rolling buffer to handle URLs split across chunks.
    // Strip all escape sequences: CSI (ESC[...), OSC (ESC]...\x07 or ESC]...ESC\\), and other ESC sequences.
    const clean = text
      .replace(/\x1b\][^\x07\x1b]*(?:\x07|\x1b\\)/g, '')  // OSC sequences (hyperlinks etc.)
      .replace(/\x1b\[[0-9;?]*[a-zA-Z]/g, '')              // CSI sequences
      .replace(/\x1b[^[\]].?/g, '');                        // Other short ESC sequences
    urlBufferRef.current = (urlBufferRef.current + clean).slice(-2000); // Keep last 2000 chars
    const match = urlBufferRef.current.match(OAUTH_URL_RE);
    if (match) {
      window.dispatchEvent(new CustomEvent('auth-url-detected', { detail: match[0] }));
      urlBufferRef.current = ''; // Clear after finding to avoid re-firing
    }

    // Debounced scan for quick-reply options (numbered lists, y/n, press Enter)
    if (quickReplyTimerRef.current) {
      clearTimeout(quickReplyTimerRef.current);
    }
    quickReplyTimerRef.current = setTimeout(() => {
      const parser = parserRef.current;
      const allLines = parser.getLines();
      const currentLine = parser.getCurrentLine();

      // Extract plain text from the last 20 lines
      const recentTermLines = allLines.slice(-19);
      if (currentLine.length > 0) {
        recentTermLines.push(currentLine);
      }

      const plainLines = recentTermLines.map(termLine =>
        termLine.map(span => span.text).join('')
      );

      const options = detectOptions(plainLines);
      const optionsJson = options ? JSON.stringify(options) : '';

      // Only dispatch if options changed (avoid unnecessary re-renders)
      if (optionsJson !== lastOptionsJsonRef.current) {
        lastOptionsJsonRef.current = optionsJson;
        if (options) {
          window.dispatchEvent(new CustomEvent<QuickReplyEventDetail>(QUICK_REPLY_EVENT, {
            detail: { options },
          }));
        } else {
          window.dispatchEvent(new CustomEvent('quick-reply-clear'));
        }
      }
    }, 300); // 300ms debounce
  }, []);

  const { state, disconnectedAt, send, sendResize, forceReconnect } = useWebSocket({
    onData: handleData,
    onConnect: onHapticConnect,
    onDisconnect: onHapticDisconnect,
  });

  sendRef.current = send;

  // Expose forceReconnect to parent
  useEffect(() => {
    onForceReconnectReady?.(forceReconnect);
  }, [forceReconnect, onForceReconnectReady]);

  // Propagate disconnectedAt to parent
  useEffect(() => {
    onDisconnectedAtChange?.(disconnectedAt);
  }, [disconnectedAt, onDisconnectedAtChange]);

  // Calculate terminal dimensions from container element
  const calcSize = useCallback((el: HTMLElement) => {
    const charWidth = 8.4;  // approx for monospace at 14px
    const lineHeight = 18.9; // 14px * 1.35 line-height
    const cols = Math.floor((el.clientWidth - 16) / charWidth) || 80;
    const rows = Math.floor(el.clientHeight / lineHeight) || 24;
    return { cols, rows };
  }, []);

  // Send initial size when connected
  useEffect(() => {
    if (state === 'connected') {
      const el = scrollRef.current;
      if (el) {
        const { cols, rows } = calcSize(el);
        sendResize(cols, rows);
      }
    }
  }, [state, sendResize, calcSize]);

  // Resize PTY when container size changes (viewport resize, keyboard open/close, rotation)
  useEffect(() => {
    const el = scrollRef.current;
    if (!el) return;

    let debounceTimer: ReturnType<typeof setTimeout> | null = null;

    const observer = new ResizeObserver(() => {
      if (debounceTimer) clearTimeout(debounceTimer);
      debounceTimer = setTimeout(() => {
        if (state !== 'connected') return;
        const { cols, rows } = calcSize(el);
        sendResize(cols, rows);
      }, 150);
    });

    observer.observe(el);

    return () => {
      observer.disconnect();
      if (debounceTimer) clearTimeout(debounceTimer);
    };
  }, [state, sendResize, calcSize]);

  // Propagate connection state
  useEffect(() => {
    onConnectionChange(state);
  }, [state, onConnectionChange]);

  // Expose WebSocket send for KeyBar, InputBar, CommandDrawer
  // Wraps send to track user input for activity detection.
  useEffect(() => {
    const globals = window as unknown as Record<string, unknown>;
    globals.__wsSend = (data: string) => {
      sendRef.current(data);

      // Track user input: after user sends something, if no output within 1s, set "waiting"
      userSentTimeRef.current = Date.now();
      // Only transition to waiting if we're currently idle (not already streaming)
      if (activityRef.current === 'idle') {
        if (activityTimerRef.current) clearTimeout(activityTimerRef.current);
        activityTimerRef.current = setTimeout(() => {
          // If still no data received since user sent, we're waiting
          if (lastDataTimeRef.current < userSentTimeRef.current) {
            setActivity('waiting');
          }
        }, 1000);
      }
    };
    return () => { delete globals.__wsSend; };
  }, [setActivity]);

  // Desktop: direct keyboard input on the terminal div.
  // Handles all keys so desktop users can type directly without InputBar.
  const handleKeyDown = useCallback((e: React.KeyboardEvent) => {
    // Don't intercept if an input/textarea is focused (InputBar handles those)
    if (document.activeElement instanceof HTMLInputElement ||
        document.activeElement instanceof HTMLTextAreaElement) return;

    const send = sendRef.current;
    if (!send) return;

    // Modifier-only keys: don't send
    if (['Shift', 'Control', 'Alt', 'Meta', 'CapsLock'].includes(e.key)) return;

    e.preventDefault();

    // Ctrl+key combos
    if (e.ctrlKey && e.key.length === 1) {
      const code = e.key.toLowerCase().charCodeAt(0) - 96;
      if (code >= 1 && code <= 26) { send(String.fromCharCode(code)); return; }
    }

    // Alt+key
    if (e.altKey && e.key.length === 1) {
      send('\x1b' + e.key);
      return;
    }

    switch (e.key) {
      case 'Enter':     send('\r'); break;
      case 'Backspace': send('\x7f'); break;
      case 'Tab':       send('\t'); break;
      case 'Escape':    send('\x1b'); break;
      case 'ArrowUp':   send('\x1b[A'); break;
      case 'ArrowDown': send('\x1b[B'); break;
      case 'ArrowLeft': send('\x1b[D'); break;
      case 'ArrowRight':send('\x1b[C'); break;
      case 'Home':      send('\x1b[H'); break;
      case 'End':       send('\x1b[F'); break;
      case 'Delete':    send('\x1b[3~'); break;
      case 'PageUp':    send('\x1b[5~'); break;
      case 'PageDown':  send('\x1b[6~'); break;
      default:
        // Printable characters (single character, no ctrl/alt modifier)
        if (e.key.length === 1 && !e.ctrlKey && !e.altKey && !e.metaKey) {
          send(e.key);
        }
    }
  }, []);

  // Handle paste on the terminal div (desktop Ctrl+V / Cmd+V)
  const handlePaste = useCallback((e: React.ClipboardEvent) => {
    const text = e.clipboardData.getData('text/plain');
    if (text) {
      e.preventDefault();
      sendRef.current(text);
    }
  }, []);

  // Detect touch device — on touch, tap dismisses keyboard; on desktop, tap focuses terminal
  const isTouchDevice = 'ontouchstart' in window || navigator.maxTouchPoints > 0;

  // Click handler: dismiss keyboard on mobile, focus terminal on desktop
  const handleTap = useCallback(() => {
    const sel = window.getSelection();
    if (sel && sel.toString().length > 0) return; // Don't act if selecting text
    if (isTouchDevice) {
      // Mobile: dismiss keyboard
      if (document.activeElement instanceof HTMLElement) {
        document.activeElement.blur();
      }
    } else {
      // Desktop: focus terminal div for direct keyboard input
      scrollRef.current?.focus();
    }
  }, [isTouchDevice]);

  // Jump-to-bottom pill handler
  const handlePillTap = useCallback((e: React.MouseEvent) => {
    e.stopPropagation(); // Don't trigger terminal tap handler
    if (scrollRef.current) {
      scrollRef.current.scrollTo({ top: scrollRef.current.scrollHeight, behavior: 'smooth' });
    }
    autoScrollRef.current = true;
    newLinesSinceScrollRef.current = 0;
    setShowNewOutputPill(false);
  }, []);

  // Render lines
  const parser = parserRef.current;
  const lines = parser.getLines();
  const currentLine = parser.getCurrentLine();

  return (
    <div className={styles.terminalWrapper}>
      <div
        ref={scrollRef}
        className={styles.terminal}
        tabIndex={0}
        onClick={handleTap}
        onKeyDown={handleKeyDown}
        onPaste={handlePaste}
        onScroll={handleScroll}
      >
        <div className={styles.content}>
          {lines.map((line, i) => (
            <Line key={i} spans={line} />
          ))}
          {currentLine.length > 0 && <Line spans={currentLine} />}
        </div>
      </div>
      {showNewOutputPill && (
        <div className={styles.newOutputPill} onClick={handlePillTap}>
          {newLinesSinceScrollRef.current > 1
            ? `\u2193 ${newLinesSinceScrollRef.current} new lines`
            : '\u2193 New output'}
        </div>
      )}
    </div>
  );
}

/** Render a single terminal line */
function Line({ spans }: { spans: TermLine }) {
  if (spans.length === 0) return <div className={styles.line}>&nbsp;</div>;
  return (
    <div className={styles.line}>
      {spans.map((span, i) => {
        const inlineStyle = spanToStyle(span.style);
        return inlineStyle
          ? <span key={i} style={cssStringToObj(inlineStyle)}>{span.text}</span>
          : <span key={i}>{span.text}</span>;
      })}
    </div>
  );
}

/** Convert "color:red;font-weight:bold" to { color: 'red', fontWeight: 'bold' } */
function cssStringToObj(css: string): React.CSSProperties {
  const obj: Record<string, string> = {};
  for (const part of css.split(';')) {
    const [key, val] = part.split(':');
    if (key && val) {
      // Convert kebab-case to camelCase
      const camelKey = key.trim().replace(/-([a-z])/g, (_, c) => c.toUpperCase());
      obj[camelKey] = val.trim();
    }
  }
  return obj as unknown as React.CSSProperties;
}
