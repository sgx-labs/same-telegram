import { useState, useCallback, useRef } from 'react';
import type { Terminal } from '@xterm/xterm';
import styles from './KeyBar.module.css';

interface KeyBarProps {
  hapticTap?: () => void;
  hapticSelection?: () => void;
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
export default function KeyBar({ hapticTap, hapticSelection }: KeyBarProps) {
  const [ctrlActive, setCtrlActive] = useState(false);
  const [altActive, setAltActive] = useState(false);
  const repeatTimerRef = useRef<ReturnType<typeof setInterval> | null>(null);

  const getTerminal = useCallback((): Terminal | null => {
    return (window as unknown as Record<string, unknown>).__terminal as Terminal | null;
  }, []);

  const sendToTerminal = useCallback((seq: string) => {
    const term = getTerminal();
    if (!term) return;

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

    term.paste(finalSeq);
    term.focus();
  }, [getTerminal, ctrlActive, altActive]);

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
        const term = getTerminal();
        term?.paste(key.seq);
      }
    }, 80);
  }, [getTerminal]);

  const stopRepeat = useCallback(() => {
    if (repeatTimerRef.current) {
      clearInterval(repeatTimerRef.current);
      repeatTimerRef.current = null;
    }
  }, []);

  return (
    <div className={styles.keybar}>
      <div className={styles.scroll}>
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
