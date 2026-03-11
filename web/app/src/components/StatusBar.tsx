import type { ConnectionState } from '../hooks/useWebSocket';
import styles from './StatusBar.module.css';

interface StatusBarProps {
  connectionState: ConnectionState;
}

/**
 * StatusBar — minimal chrome at the top of the terminal.
 *
 * Shows connection status with a colored dot and label.
 * Animated transitions between states.
 */
export default function StatusBar({ connectionState }: StatusBarProps) {
  const statusText: Record<ConnectionState, string> = {
    connecting: 'connecting',
    connected: 'connected',
    disconnected: 'reconnecting',
  };

  return (
    <div className={styles.statusbar}>
      <div className={styles.left}>
        <span className={styles.brand}>SameVault</span>
      </div>
      <div className={styles.right}>
        <span className={`${styles.dot} ${styles[connectionState]}`} />
        <span className={`${styles.label} ${styles[connectionState]}`}>
          {statusText[connectionState]}
        </span>
      </div>
    </div>
  );
}
