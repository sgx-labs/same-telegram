import { useState, useCallback, useRef } from 'react';
import styles from './KeyBar.module.css';

/** Get the global WebSocket send function exposed by Terminal */
function getWsSendGlobal(): ((data: string) => void) | null {
  return (window as unknown as Record<string, unknown>).__wsSend as ((data: string) => void) | null;
}

interface KeyBarProps {
  hapticTap?: () => void;
  hapticSelection?: () => void;
  onCommandDrawer?: () => void;
}

interface KeyDef {
  label: string;
  /** Send this exact sequence to the terminal */
  seq?: string;
  /** Send ctrl+<key> (computes control code) */
  ctrl?: string;
  /** This is a modifier toggle (Ctrl, Alt) */
  modifier?: 'ctrl' | 'alt';
  /** Visual group separator (adds spacing before this key) */
  group?: boolean;
}

const KEYS: KeyDef[] = [
  { label: 'Esc', seq: '\x1b' },
  { label: 'Tab', seq: '\t' },
  { label: 'Ctrl', modifier: 'ctrl', group: true },
  { label: 'Alt', modifier: 'alt' },
  { label: '\u2191', seq: '\x1b[A', group: true },  // Up arrow
  { label: '\u2193', seq: '\x1b[B' },                // Down arrow
  { label: '\u2190', seq: '\x1b[D' },                // Left arrow
  { label: '\u2192', seq: '\x1b[C' },                // Right arrow
  { label: '^C', ctrl: 'c', group: true },
  { label: '^D', ctrl: 'd' },
  { label: '^Z', ctrl: 'z' },
  { label: '^L', ctrl: 'l' },
  { label: '|', seq: '|', group: true },
  { label: '~', seq: '~' },
  { label: '/', seq: '/' },
  { label: '-', seq: '-' },
];

/**
 * KeyBar — custom touch input for the terminal.
 *
 * This IS the primary input method on mobile (native keyboard is suppressed).
 * Features:
 * - Ctrl/Alt as toggle modifiers
 * - Long-press repeat on arrow keys
 * - Haptic feedback on every press
 * - Contextual visual state
 */
export default function KeyBar({ hapticTap, hapticSelection, onCommandDrawer }: KeyBarProps) {
  const [ctrlActive, setCtrlActive] = useState(false);
  const [altActive, setAltActive] = useState(false);
  const repeatTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const getWsSend = useCallback((): ((data: string) => void) | null => {
    return (window as unknown as Record<string, unknown>).__wsSend as ((data: string) => void) | null;
  }, []);

  const sendToTerminal = useCallback((seq: string) => {
    const send = getWsSend();
    if (!send) return;

    let finalSeq = seq;

    // Apply active modifiers
    if (altActive && seq.length === 1) {
      finalSeq = '\x1b' + seq; // Alt+key = ESC + key
      setAltActive(false);
    }
    if (ctrlActive && seq.length === 1) {
      // Ctrl+letter = char code 1-26
      const code = seq.toLowerCase().charCodeAt(0) - 96;
      if (code >= 1 && code <= 26) {
        finalSeq = String.fromCharCode(code);
      }
      setCtrlActive(false);
    }

    // Send directly to WebSocket (bypasses xterm.js input handler)
    send(finalSeq);
  }, [getWsSend, ctrlActive, altActive]);

  const handlePress = useCallback((key: KeyDef) => {
    hapticTap?.();

    if (key.modifier === 'ctrl') {
      setCtrlActive(prev => !prev);
      hapticSelection?.();
      return;
    }
    if (key.modifier === 'alt') {
      setAltActive(prev => !prev);
      hapticSelection?.();
      return;
    }

    if (key.ctrl) {
      const code = key.ctrl.charCodeAt(0) - 96;
      sendToTerminal(String.fromCharCode(code));
      return;
    }

    if (key.seq) {
      sendToTerminal(key.seq);
    }
  }, [hapticTap, hapticSelection, sendToTerminal]);

  // Long-press repeat for arrow keys
  const startRepeat = useCallback((key: KeyDef) => {
    if (!key.seq?.startsWith('\x1b[')) return; // Only arrows
    repeatTimerRef.current = setInterval(() => {
      if (key.seq) {
        const send = getWsSend();
        if (send) send(key.seq);
      }
    }, 80);
  }, [getWsSend]);

  const stopRepeat = useCallback(() => {
    if (repeatTimerRef.current) {
      clearInterval(repeatTimerRef.current);
      repeatTimerRef.current = null;
    }
  }, []);

  // Dismiss keyboard — use Telegram hideKeyboard API (v9.1+), fallback to blur
  const handleDismiss = useCallback(() => {
    hapticTap?.();
    const tg = window.Telegram?.WebApp;
    if (tg && 'hideKeyboard' in tg) {
      (tg as unknown as { hideKeyboard: () => void }).hideKeyboard();
    }
    if (document.activeElement instanceof HTMLElement) {
      document.activeElement.blur();
    }
  }, [hapticTap]);

  // Paste from clipboard into terminal — try multiple approaches
  const handlePaste = useCallback(async () => {
    hapticTap?.();
    const send = getWsSendGlobal();
    if (!send) return;

    // 1. Try Telegram's readTextFromClipboard (works in attachment menu Mini Apps)
    const tg = window.Telegram?.WebApp;
    if (tg && 'readTextFromClipboard' in tg) {
      (tg as unknown as { readTextFromClipboard: (cb: (text: string | null) => void) => void })
        .readTextFromClipboard((text) => {
          if (text) send(text);
        });
      return;
    }

    // 2. Try browser Clipboard API
    try {
      const text = await navigator.clipboard.readText();
      if (text) { send(text); return; }
    } catch { /* blocked in WebView — expected */ }

    // 3. Fallback: focus a temporary textarea so user can long-press paste
    const ta = document.createElement('textarea');
    ta.style.cssText = 'position:fixed;top:50%;left:10%;width:80%;z-index:9999;font-size:16px;padding:12px;border-radius:8px;background:#1a1b26;color:#a9b1d6;border:1px solid #7aa2f7;';
    ta.placeholder = 'Long-press here to paste, then tap Done';
    document.body.appendChild(ta);
    ta.focus();
    const cleanup = () => {
      if (ta.value) send(ta.value);
      ta.remove();
    };
    ta.addEventListener('blur', cleanup, { once: true });
    // Auto-remove after 10s
    setTimeout(() => { if (ta.parentNode) cleanup(); }, 10000);
  }, [hapticTap]);

  return (
    <div className={styles.keybar}>
      <div className={styles.scroll}>
        {onCommandDrawer && (
          <button
            className={`${styles.key} ${styles.menuBtn}`}
            onPointerDown={(e) => {
              e.preventDefault();
              hapticTap?.();
              onCommandDrawer();
            }}
            onContextMenu={(e) => e.preventDefault()}
          >
            +
          </button>
        )}
        <button
          className={styles.key}
          onPointerDown={(e) => {
            e.preventDefault();
            handlePaste();
          }}
          onContextMenu={(e) => e.preventDefault()}
        >
          Paste
        </button>
        <button
          className={styles.key}
          onPointerDown={(e) => {
            e.preventDefault();
            handleDismiss();
          }}
          onContextMenu={(e) => e.preventDefault()}
        >
          ▼
        </button>
        {KEYS.map((key, i) => {
          const isActive =
            (key.modifier === 'ctrl' && ctrlActive) ||
            (key.modifier === 'alt' && altActive);

          return (
            <button
              key={i}
              className={`${styles.key} ${isActive ? styles.active : ''} ${key.group ? styles.group : ''}`}
              onPointerDown={(e) => {
                e.preventDefault();
                handlePress(key);
                startRepeat(key);
              }}
              onPointerUp={stopRepeat}
              onPointerLeave={stopRepeat}
              onContextMenu={(e) => e.preventDefault()}
            >
              {key.label}
            </button>
          );
        })}
      </div>
    </div>
  );
}
