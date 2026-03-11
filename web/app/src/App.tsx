import { useState, useCallback } from 'react';
import Terminal from './components/Terminal';
import StatusBar from './components/StatusBar';
import KeyBar from './components/KeyBar';
import { useTelegram } from './hooks/useTelegram';
import type { ConnectionState } from './hooks/useWebSocket';

/**
 * SameVault Terminal — Root Application
 *
 * Layout: StatusBar (top) → Terminal (flex fill) → KeyBar (bottom)
 * Full viewport, no scroll, every pixel used.
 */
export default function App() {
  const { haptic, isInTelegram } = useTelegram();
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting');

  const handleConnectionChange = useCallback((state: ConnectionState) => {
    setConnectionState(state);
  }, []);

  const handleConnect = useCallback(() => {
    haptic.success();
  }, [haptic]);

  const handleDisconnect = useCallback(() => {
    haptic.error();
  }, [haptic]);

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: '100%',
      width: '100%',
      overflow: 'hidden',
    }}>
      {/* Hide status bar inside Telegram — its native header shows "SameVault" already */}
      {!isInTelegram && <StatusBar connectionState={connectionState} />}
      <Terminal
        onConnectionChange={handleConnectionChange}
        onHapticConnect={handleConnect}
        onHapticDisconnect={handleDisconnect}
      />
      <KeyBar
        hapticTap={haptic.tap}
        hapticSelection={haptic.selection}
      />
    </div>
  );
}
