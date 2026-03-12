import { useEffect, useState } from 'react';
import type { ConnectionState } from '../hooks/useWebSocket';
import type { TerminalActivity } from './HtmlTerminal';
import styles from './StatusBar.module.css';

interface StatusBarProps {
  connectionState: ConnectionState;
  disconnectedAt?: number | null;
  onReconnect?: () => void;
  activity?: TerminalActivity;
}

/**
 * StatusBar — minimal chrome at the top of the terminal.
 *
 * Shows connection status with a colored dot and label.
 * When disconnected for >10s, the bar becomes tappable to force a reconnect.
 * Animated transitions between states.
 */
export default function StatusBar({ connectionState, disconnectedAt, onReconnect, activity = 'idle' }: StatusBarProps) {
  const [showTapHint, setShowTapHint] = useState(false);

  // After 10s disconnected, show the tap-to-reconnect hint
  useEffect(() => {
    if (connectionState !== 'disconnected' || !disconnectedAt) {
      setShowTapHint(false);
      return;
    }

    const elapsed = Date.now() - disconnectedAt;
    const remaining = Math.max(0, 10000 - elapsed);

    const timer = setTimeout(() => {
      setShowTapHint(true);
    }, remaining);

    return () => clearTimeout(timer);
  }, [connectionState, disconnectedAt]);

  const handleTap = () => {
    if (showTapHint && onReconnect) {
      onReconnect();
      setShowTapHint(false);
    }
  };

  const statusText: Record<ConnectionState, string> = {
    connecting: 'connecting',
    connected: 'connected',
    disconnected: 'reconnecting...',
  };

  // Determine the effective dot/label style class when connected
  // Activity states override the default "connected" appearance
  const getStatusClass = () => {
    if (connectionState !== 'connected') return connectionState;
    if (activity === 'waiting') return 'waiting';
    if (activity === 'streaming') return 'streaming';
    return connectionState; // idle -> normal connected
  };

  const getStatusText = () => {
    if (connectionState !== 'connected') {
      return isTappable ? 'tap to reconnect' : statusText[connectionState];
    }
    if (activity === 'waiting') return 'thinking...';
    if (activity === 'streaming') return 'responding...';
    return 'connected';
  };

  const isTappable = showTapHint && connectionState === 'disconnected';
  const statusClass = getStatusClass();

  return (
    <div
      className={`${styles.statusbar} ${isTappable ? styles.tappable : ''}`}
      onClick={isTappable ? handleTap : undefined}
      role={isTappable ? 'button' : undefined}
      tabIndex={isTappable ? 0 : undefined}
    >
      <div className={styles.left}>
        <span className={styles.brand}>SameVault</span>
      </div>
      <div className={styles.right}>
        <span className={`${styles.dot} ${styles[statusClass]}`} />
        <span className={`${styles.label} ${styles[statusClass]}`}>
          {getStatusText()}
        </span>
      </div>
    </div>
  );
}
