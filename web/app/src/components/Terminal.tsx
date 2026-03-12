import { useEffect, useRef, useCallback } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { CanvasAddon } from '@xterm/addon-canvas';
import '@xterm/xterm/css/xterm.css';

import { useWebSocket, type ConnectionState } from '../hooks/useWebSocket';

/** Telegram WebApp for viewport events */
const tg = typeof window !== 'undefined' ? window.Telegram?.WebApp : undefined;

/** Terminal padding in px — gives text breathing room (Hyper-style) */
const TERMINAL_PAD_X = 8;
const TERMINAL_PAD_Y = 4;

interface TerminalProps {
  onConnectionChange: (state: ConnectionState) => void;
  onHapticConnect?: () => void;
  onHapticDisconnect?: () => void;
}

/**
 * Terminal component — xterm.js with WebSocket relay.
 *
 * Handles:
 * - Terminal initialization and theming
 * - WebSocket data flow (binary I/O)
 * - Debounced resize with Telegram viewport awareness
 * - Mobile keyboard input with autocorrect disabled
 * - Canvas rendering for GPU acceleration
 */
/** Strip ANSI escape sequences from text */
function stripAnsi(s: string): string {
  return s.replace(/\x1b\[[0-9;]*[a-zA-Z]|\x1b\][^\x07]*\x07|\x1b[()][0-9A-B]/g, '');
}

/** URL pattern for Claude OAuth */
const OAUTH_URL_RE = /https:\/\/claude\.ai\/oauth\/authorize[^\s\x1b\x07\x00-\x1f]*/;

export default function Terminal({ onConnectionChange, onHapticConnect, onHapticDisconnect }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const resizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const sendRef = useRef<(data: string | Uint8Array) => void>(() => {});
  const urlBufferRef = useRef('');

  // Terminal write callback for WebSocket — also scans for OAuth URLs
  const handleData = useCallback((data: ArrayBuffer) => {
    termRef.current?.write(new Uint8Array(data));

    // Scan for OAuth URLs in terminal output
    const text = new TextDecoder().decode(data);
    // Keep a rolling buffer (last 4KB) to catch URLs split across chunks
    urlBufferRef.current = (urlBufferRef.current + text).slice(-4096);
    const clean = stripAnsi(urlBufferRef.current);
    const match = clean.match(OAUTH_URL_RE);
    if (match) {
      const url = match[0];
      // Dispatch custom event for AuthBanner to pick up
      window.dispatchEvent(new CustomEvent('auth-url-detected', { detail: url }));
      // Clear buffer so we don't re-fire for the same URL
      urlBufferRef.current = '';
    }
  }, []);

  const { state, send, sendResize } = useWebSocket({
    onData: handleData,
    onConnect: onHapticConnect,
    onDisconnect: onHapticDisconnect,
  });

  // Keep send ref current so the onData closure in the useEffect always
  // calls the latest send function (avoids stale closure).
  sendRef.current = send;

  // Re-fit terminal when WebSocket connects so server gets correct dimensions
  useEffect(() => {
    if (state === 'connected' && fitAddonRef.current && termRef.current) {
      setTimeout(() => {
        fitAddonRef.current?.fit();
        const t = termRef.current;
        if (t) sendResize(t.cols, t.rows);
      }, 50);
    }
  }, [state, sendResize]);

  // Propagate connection state
  useEffect(() => {
    onConnectionChange(state);
  }, [state, onConnectionChange]);

  // Initialize terminal
  useEffect(() => {
    if (!containerRef.current || termRef.current) return;

    const isMobile = /Android|iPhone|iPad/i.test(navigator.userAgent);

    const term = new XTerm({
      fontFamily: "'JetBrains Mono', 'Fira Code', 'SF Mono', 'Consolas', monospace",
      fontSize: isMobile ? 15 : 14,
      lineHeight: 1.25,
      letterSpacing: 0,
      cursorBlink: true,
      cursorStyle: 'block',
      scrollback: 5000,
      allowProposedApi: true,
      theme: getTerminalTheme(),
    });

    const fitAddon = new FitAddon();
    term.loadAddon(fitAddon);
    term.loadAddon(new WebLinksAddon());

    term.open(containerRef.current);

    // Load canvas addon for GPU-accelerated rendering (critical for mobile perf)
    try {
      term.loadAddon(new CanvasAddon());
    } catch {
      // Canvas addon may fail in some environments — WebGL fallback is fine
    }

    // On mobile, allow native keyboard but disable autocorrect/autocapitalize
    // which interfere with terminal input. The KeyBar provides supplementary
    // special keys (Ctrl, Esc, arrows) alongside the native keyboard.
    if (isMobile) {
      const textarea = containerRef.current.querySelector('.xterm-helper-textarea') as HTMLTextAreaElement;
      if (textarea) {
        textarea.setAttribute('autocomplete', 'off');
        textarea.setAttribute('autocorrect', 'off');
        textarea.setAttribute('autocapitalize', 'off');
        textarea.setAttribute('spellcheck', 'false');
        textarea.setAttribute('inputmode', 'text');
      }
    }

    // Send terminal input to WebSocket via ref (never stale).
    term.onData((data) => sendRef.current(data));

    // Don't auto-focus xterm — InputBar is the primary input on mobile.
    // Focusing xterm steals focus from InputBar and fights with it on resize.

    termRef.current = term;
    fitAddonRef.current = fitAddon;

    // Fit + sync: fit the terminal and send dimensions to server
    const fitAndSync = () => {
      if (!fitAddonRef.current || !termRef.current) return;
      fitAddonRef.current.fit();
      const t = termRef.current;
      sendResize(t.cols, t.rows);
    };

    // Telegram expand()/requestFullscreen() are async.
    // Fit multiple times to catch the final viewport size.
    requestAnimationFrame(fitAndSync);
    const t1 = setTimeout(fitAndSync, 200);
    const t2 = setTimeout(fitAndSync, 600);
    const t3 = setTimeout(fitAndSync, 1200);

    // Debounced resize handler
    const handleResize = () => {
      if (resizeTimerRef.current) clearTimeout(resizeTimerRef.current);
      resizeTimerRef.current = setTimeout(fitAndSync, 80);
    };

    window.addEventListener('resize', handleResize);

    // Listen for Telegram viewport changes (expand, keyboard, etc.)
    // viewportChanged fires on keyboard open/close and fullscreen transitions.
    if (tg) {
      tg.onEvent('viewportChanged', handleResize);
    }

    // Also listen for visual viewport resize (keyboard on iOS/Android)
    const vv = window.visualViewport;
    if (vv) {
      vv.addEventListener('resize', handleResize);
    }

    // Expose terminal and WebSocket send for KeyBar, InputBar, CommandDrawer.
    const globals = window as unknown as Record<string, unknown>;
    globals.__terminal = term;
    globals.__wsSend = (data: string) => sendRef.current(data);

    return () => {
      window.removeEventListener('resize', handleResize);
      if (tg) tg.offEvent('viewportChanged', handleResize);
      if (vv) vv.removeEventListener('resize', handleResize);
      clearTimeout(t1);
      clearTimeout(t2);
      clearTimeout(t3);
      if (resizeTimerRef.current) clearTimeout(resizeTimerRef.current);
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
      delete globals.__terminal;
      delete globals.__wsSend;
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  // Tap-to-dismiss: tapping the terminal output area dismisses the keyboard.
  // This is the primary keyboard dismiss mechanism — user taps output to
  // close keyboard and see results, taps InputBar to type again.
  const handleTap = useCallback(() => {
    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur();
    }
  }, []);

  return (
    <div
      ref={containerRef}
      onClickCapture={handleTap}
      style={{
        flex: 1,
        minHeight: 0,
        overflow: 'hidden',
        background: 'var(--sv-terminal-bg)',
        padding: `${TERMINAL_PAD_Y}px ${TERMINAL_PAD_X}px`,
      }}
    />
  );
}

/**
 * Build xterm.js theme from our CSS custom properties.
 * Falls back to Tokyo Night colors when not in Telegram.
 */
function getTerminalTheme() {
  const style = getComputedStyle(document.documentElement);
  const get = (prop: string, fallback: string) =>
    style.getPropertyValue(prop).trim() || fallback;

  return {
    background: get('--sv-terminal-bg', '#1a1b26'),
    foreground: get('--sv-terminal-fg', '#a9b1d6'),
    cursor: get('--sv-terminal-cursor', '#c0caf5'),
    cursorAccent: get('--sv-terminal-bg', '#1a1b26'),
    selectionBackground: get('--sv-terminal-selection', '#33467c'),
    // Tokyo Night ANSI colors
    black: '#15161e',
    red: '#f7768e',
    green: '#9ece6a',
    yellow: '#e0af68',
    blue: '#7aa2f7',
    magenta: '#bb9af7',
    cyan: '#7dcfff',
    white: '#a9b1d6',
    brightBlack: '#414868',
    brightRed: '#f7768e',
    brightGreen: '#9ece6a',
    brightYellow: '#e0af68',
    brightBlue: '#7aa2f7',
    brightMagenta: '#bb9af7',
    brightCyan: '#7dcfff',
    brightWhite: '#c0caf5',
  };
}
