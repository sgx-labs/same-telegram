import { useRef, useEffect, useCallback, useState } from 'react';
import styles from './InputBar.module.css';

/** Get the global WebSocket send function exposed by Terminal */
function getWsSend(): ((data: string) => void) | null {
  return (window as unknown as Record<string, unknown>).__wsSend as ((data: string) => void) | null;
}

interface InputBarProps {
  hapticTap?: () => void;
}

/**
 * InputBar — line-buffered text input for terminal keyboard capture.
 *
 * Uses native DOM event listeners instead of React synthetic events for
 * maximum compatibility with Telegram's WKWebView on iOS.
 *
 * How it works:
 * - User taps the input field -> native keyboard appears
 * - Typed text accumulates visibly in the input (line buffer)
 * - Enter / Send submits the buffered line to WebSocket -> PTY -> tmux
 * - The input is cleared only after submission
 * - Special keys (Tab, arrows, Escape) flush any buffered text first
 * - Also handles IME composition (compositionend) for CJK / dictation
 */
export default function InputBar({ hapticTap }: InputBarProps) {
  const inputRef = useRef<HTMLInputElement>(null);
  const hapticRef = useRef(hapticTap);
  hapticRef.current = hapticTap;
  const [focused, setFocused] = useState(false);

  // Attach native DOM event listeners for maximum WebView compatibility.
  // React synthetic events (onInput, onKeyDown) don't fire reliably in
  // Telegram's WKWebView on iOS.
  useEffect(() => {
    const input = inputRef.current;
    if (!input) return;

    const handleInput = () => {
      // Line-buffer mode: let text accumulate in the input field.
      // Text is sent only on Enter, Tab, arrow keys, or other special keys.
      // No-op here — the native input handles display.
    };

    const handleKeyDown = (e: KeyboardEvent) => {
      const send = getWsSend();
      if (!send) return;

      switch (e.key) {
        case 'Enter':
          e.preventDefault();
          // Send any buffered text first, then carriage return
          if (input.value) {
            send(input.value);
            input.value = '';
          }
          send('\r');
          hapticRef.current?.();
          return;
        case 'Backspace':
          if (input.value === '') {
            e.preventDefault();
            send('\x7f');
          }
          break;
        case 'Tab':
          e.preventDefault();
          // Flush any buffered text, then send tab for completion
          if (input.value) {
            send(input.value);
            input.value = '';
          }
          send('\t');
          break;
        case 'Escape':
          e.preventDefault();
          send('\x1b');
          input.value = '';
          break;
        case 'ArrowUp':
          e.preventDefault();
          if (input.value) {
            send(input.value);
            input.value = '';
          }
          send('\x1b[A');
          break;
        case 'ArrowDown':
          e.preventDefault();
          if (input.value) {
            send(input.value);
            input.value = '';
          }
          send('\x1b[B');
          break;
        case 'ArrowLeft':
          e.preventDefault();
          if (input.value) {
            send(input.value);
            input.value = '';
          }
          send('\x1b[D');
          break;
        case 'ArrowRight':
          e.preventDefault();
          if (input.value) {
            send(input.value);
            input.value = '';
          }
          send('\x1b[C');
          break;
      }
    };

    // IME composition handlers (CJK input, dictation, iOS autocomplete)
    // In line-buffer mode, composed text accumulates in the input like any
    // other text, so these are no-ops. We still listen to prevent default
    // browser behavior from interfering.
    const handleCompositionStart = () => { /* no-op */ };
    const handleCompositionEnd = () => { /* no-op */ };

    // Native paste event — catches long-press paste and Cmd+V
    const handlePaste = (e: ClipboardEvent) => {
      const text = e.clipboardData?.getData('text/plain');
      if (text) {
        e.preventDefault();
        const send = getWsSend();
        if (send) send(text);
      }
    };

    const handleFocus = () => setFocused(true);
    const handleBlur = () => setFocused(false);

    input.addEventListener('input', handleInput);
    input.addEventListener('keydown', handleKeyDown);
    input.addEventListener('compositionstart', handleCompositionStart);
    input.addEventListener('compositionend', handleCompositionEnd);
    input.addEventListener('paste', handlePaste);
    input.addEventListener('focus', handleFocus);
    input.addEventListener('blur', handleBlur);

    return () => {
      input.removeEventListener('input', handleInput);
      input.removeEventListener('keydown', handleKeyDown);
      input.removeEventListener('compositionstart', handleCompositionStart);
      input.removeEventListener('compositionend', handleCompositionEnd);
      input.removeEventListener('paste', handlePaste);
      input.removeEventListener('focus', handleFocus);
      input.removeEventListener('blur', handleBlur);
    };
  }, []);

  // Submit handler — most reliable way to catch Enter/Send on iOS.
  // The software keyboard's Send button submits the enclosing <form>.
  const handleSubmit = useCallback((e: React.FormEvent) => {
    e.preventDefault();
    const send = getWsSend();
    if (!send) return;

    // Send any remaining text in the input first
    const input = inputRef.current;
    if (input && input.value) {
      send(input.value);
      input.value = '';
    }

    send('\r');
    hapticRef.current?.();
  }, []);

  // Focus the input when the user taps the bar area
  const handleTap = useCallback(() => {
    inputRef.current?.focus();
  }, []);

  // Dismiss keyboard without submitting
  const handleDismiss = useCallback((e: React.PointerEvent) => {
    e.preventDefault();
    e.stopPropagation();
    inputRef.current?.blur();
  }, []);


  return (
    <form className={styles.inputBar} onSubmit={handleSubmit} onPointerDown={handleTap}>
      <span className={styles.prompt}>&gt;</span>
      <input
        ref={inputRef}
        className={styles.input}
        type="text"
        autoComplete="off"
        autoCorrect="off"
        autoCapitalize="off"
        spellCheck={false}
        inputMode="text"
        enterKeyHint="send"
        placeholder="type a command..."
      />
      {focused && (
        <button
          type="button"
          className={styles.dismissBtn}
          onPointerDown={handleDismiss}
          tabIndex={-1}
          aria-label="Hide keyboard"
        >
          ▼
        </button>
      )}
    </form>
  );
}
