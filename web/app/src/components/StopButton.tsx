import { useState, useEffect } from 'react';
import type { TerminalActivity } from './HtmlTerminal';
import styles from './StopButton.module.css';

/** Get the global WebSocket send function exposed by HtmlTerminal */
function getWsSend(): ((data: string) => void) | null {
  return (window as unknown as Record<string, unknown>).__wsSend as ((data: string) => void) | null;
}

interface StopButtonProps {
  activity: TerminalActivity;
  hapticTap?: () => void;
}

/**
 * StopButton — floating red panic button to interrupt Claude.
 *
 * Appears when the terminal is streaming or waiting (Claude is responding or thinking).
 * Sends Ctrl+C (\x03) via the WebSocket to interrupt the running process.
 * Positioned floating above the InputBar, centered, with fade in/out animation.
 */
export default function StopButton({ activity, hapticTap }: StopButtonProps) {
  const isActive = activity === 'streaming' || activity === 'waiting';
  // Track mount state so we can animate out before hiding
  const [mounted, setMounted] = useState(false);

  useEffect(() => {
    if (isActive) {
      setMounted(true);
    } else if (mounted) {
      // Keep mounted briefly for fade-out animation
      const timer = setTimeout(() => setMounted(false), 250);
      return () => clearTimeout(timer);
    }
  }, [isActive, mounted]);

  if (!mounted) return null;

  const handleStop = () => {
    const send = getWsSend();
    if (send) {
      send('\x03'); // Ctrl+C
    }
    hapticTap?.();
  };

  return (
    <div className={styles.wrapper}>
      <button
        className={`${styles.button} ${isActive ? styles.visible : styles.hidden}`}
        onClick={handleStop}
        type="button"
        aria-label="Stop"
      >
        <span className={styles.icon}>&#9632;</span>
        Stop
      </button>
    </div>
  );
}
