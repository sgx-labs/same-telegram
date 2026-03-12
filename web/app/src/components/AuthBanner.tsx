import { useState, useEffect, useCallback } from 'react';
import styles from './AuthBanner.module.css';

interface AuthBannerProps {
  hapticTap?: () => void;
}

/** Get the global WebSocket send function exposed by Terminal */
function getWsSend(): ((data: string) => void) | null {
  return (window as unknown as Record<string, unknown>).__wsSend as ((data: string) => void) | null;
}

/**
 * AuthBanner — floating overlay for OAuth login flow.
 *
 * Listens for 'auth-url-detected' custom events from Terminal.
 * Shows a prominent "Open in Browser" button so users can complete
 * OAuth without trying to copy URLs from the terminal.
 * Also provides a "Paste Code" button for the return flow.
 */
export default function AuthBanner({ hapticTap }: AuthBannerProps) {
  const [authUrl, setAuthUrl] = useState<string | null>(null);
  const [dismissed, setDismissed] = useState(false);
  const [pasting, setPasting] = useState(false);

  useEffect(() => {
    const handler = (e: Event) => {
      const url = (e as CustomEvent).detail as string;
      setAuthUrl(url);
      setDismissed(false);
    };
    window.addEventListener('auth-url-detected', handler);
    return () => window.removeEventListener('auth-url-detected', handler);
  }, []);

  const handleOpen = useCallback(() => {
    if (!authUrl) return;
    hapticTap?.();

    // Use Telegram's openLink for external browser, fall back to window.open
    const tg = window.Telegram?.WebApp;
    if (tg && 'openLink' in tg) {
      (tg as unknown as { openLink: (url: string) => void }).openLink(authUrl);
    } else {
      window.open(authUrl, '_blank');
    }
  }, [authUrl, hapticTap]);

  const handlePaste = useCallback(async () => {
    hapticTap?.();
    setPasting(true);
    const send = getWsSend();
    if (!send) { setPasting(false); return; }

    // 1. Try Telegram's readTextFromClipboard
    const tg = window.Telegram?.WebApp;
    if (tg && 'readTextFromClipboard' in tg) {
      (tg as unknown as { readTextFromClipboard: (cb: (text: string | null) => void) => void })
        .readTextFromClipboard((text) => {
          if (text) send(text);
          setPasting(false);
        });
      return;
    }

    // 2. Try browser Clipboard API
    try {
      const text = await navigator.clipboard.readText();
      if (text) send(text);
      setPasting(false);
      return;
    } catch { /* expected in WebView */ }

    // 3. Fallback: show temporary textarea for long-press paste
    setPasting(false);
    const ta = document.createElement('textarea');
    ta.style.cssText = 'position:fixed;top:50%;left:10%;width:80%;z-index:9999;font-size:16px;padding:12px;border-radius:8px;background:#1a1b26;color:#a9b1d6;border:1px solid #7aa2f7;';
    ta.placeholder = 'Long-press here to paste, then tap away';
    document.body.appendChild(ta);
    ta.focus();
    const cleanup = () => {
      if (ta.value) send(ta.value);
      ta.remove();
    };
    ta.addEventListener('blur', cleanup, { once: true });
    setTimeout(() => { if (ta.parentNode) cleanup(); }, 10000);
  }, [hapticTap]);

  const handleDismiss = useCallback(() => {
    setDismissed(true);
  }, []);

  if (!authUrl || dismissed) return null;

  return (
    <div className={styles.banner}>
      <div className={styles.header}>
        <span className={styles.title}>Sign in to Claude</span>
        <button className={styles.closeBtn} onClick={handleDismiss} type="button">&times;</button>
      </div>
      <p className={styles.hint}>Open the link in your browser to authenticate, then paste the code back here.</p>
      <div className={styles.actions}>
        <button className={styles.primaryBtn} onClick={handleOpen} type="button">
          Open in Browser
        </button>
        <button className={styles.secondaryBtn} onClick={handlePaste} type="button" disabled={pasting}>
          {pasting ? 'Pasting...' : 'Paste Code'}
        </button>
      </div>
    </div>
  );
}
