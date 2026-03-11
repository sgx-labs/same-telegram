import { useEffect, useRef, useCallback } from 'react';
import { Terminal as XTerm } from '@xterm/xterm';
import { FitAddon } from '@xterm/addon-fit';
import { WebLinksAddon } from '@xterm/addon-web-links';
import { CanvasAddon } from '@xterm/addon-canvas';
import '@xterm/xterm/css/xterm.css';

import { useWebSocket, type ConnectionState } from '../hooks/useWebSocket';

/** Telegram WebApp for viewport events */
const tg = typeof window !== 'undefined' ? window.Telegram?.WebApp : undefined;

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
 * - Debounced resize with mobile column cap
 * - Mobile keyboard suppression
 * - Canvas rendering for GPU acceleration
 */
export default function Terminal({ onConnectionChange, onHapticConnect, onHapticDisconnect }: TerminalProps) {
  const containerRef = useRef<HTMLDivElement>(null);
  const termRef = useRef<XTerm | null>(null);
  const fitAddonRef = useRef<FitAddon | null>(null);
  const resizeTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Terminal write callback for WebSocket
  const handleData = useCallback((data: ArrayBuffer) => {
    termRef.current?.write(new Uint8Array(data));
  }, []);

  const { state, send, sendResize } = useWebSocket({
    onData: handleData,
    onConnect: onHapticConnect,
    onDisconnect: onHapticDisconnect,
  });

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
      fontFamily: "var(--sv-font-mono), 'JetBrains Mono', monospace",
      fontSize: isMobile ? 13 : 14,
      lineHeight: 1.2,
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
      }
    }

    // Send terminal input to WebSocket
    term.onData((data) => send(data));

    termRef.current = term;
    fitAddonRef.current = fitAddon;

    // Fit + sync: fit the terminal and send dimensions to server
    const fitAndSync = () => {
      if (!fitAddonRef.current || !termRef.current) return;
      fitAddonRef.current.fit();
      const t = termRef.current;
      sendResize(t.cols, t.rows);
    };

    // Telegram expand() is async. Fit multiple times to catch the final viewport.
    requestAnimationFrame(fitAndSync);
    const t1 = setTimeout(fitAndSync, 300);
    const t2 = setTimeout(fitAndSync, 800);
    const t3 = setTimeout(fitAndSync, 1500);

    // Debounced resize handler
    const handleResize = () => {
      if (resizeTimerRef.current) clearTimeout(resizeTimerRef.current);
      resizeTimerRef.current = setTimeout(fitAndSync, 100);
    };

    window.addEventListener('resize', handleResize);

    // Listen for Telegram viewport changes (expand, keyboard, etc.)
    if (tg) {
      tg.onEvent('viewportChanged', handleResize);
    }

    // Expose for key bar
    (window as unknown as Record<string, unknown>).__terminal = term;

    return () => {
      window.removeEventListener('resize', handleResize);
      if (tg) tg.offEvent('viewportChanged', handleResize);
      clearTimeout(t1);
      clearTimeout(t2);
      clearTimeout(t3);
      if (resizeTimerRef.current) clearTimeout(resizeTimerRef.current);
      term.dispose();
      termRef.current = null;
      fitAddonRef.current = null;
      delete (window as unknown as Record<string, unknown>).__terminal;
    };
  }, []); // eslint-disable-line react-hooks/exhaustive-deps

  return (
    <div
      ref={containerRef}
      style={{
        flex: 1,
        overflow: 'hidden',
        background: 'var(--sv-terminal-bg)',
        zIndex: 'var(--sv-z-terminal)',
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
