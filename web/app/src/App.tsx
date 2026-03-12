import { useState, useCallback, useRef } from 'react';
import HtmlTerminal from './components/HtmlTerminal';
import type { TerminalActivity } from './components/HtmlTerminal';
import StatusBar from './components/StatusBar';
import InputBar from './components/InputBar';
import KeyBar from './components/KeyBar';
import CommandDrawer from './components/CommandDrawer';
import QuickReply from './components/QuickReply';
import AuthBanner from './components/AuthBanner';
import StopButton from './components/StopButton';
import { useTelegram } from './hooks/useTelegram';
import type { ConnectionState } from './hooks/useWebSocket';

/**
 * SameVault Terminal — Root Application
 *
 * Layout (top to bottom):
 *   StatusBar (non-Telegram only)
 *   Terminal  (flex fill — HTML terminal display)
 *   InputBar  (visible text input — captures keyboard on mobile)
 *   KeyBar    (special keys: Esc, Tab, Ctrl, arrows, etc.)
 *
 * The InputBar is the primary keyboard input method. On Telegram's WebView,
 * the xterm.js hidden textarea doesn't reliably trigger the keyboard.
 * InputBar is a real visible <input> that iOS/Android will show the
 * keyboard for. Keystrokes are forwarded directly to the WebSocket.
 */
export default function App() {
  const { haptic } = useTelegram();
  const [connectionState, setConnectionState] = useState<ConnectionState>('connecting');
  const [disconnectedAt, setDisconnectedAt] = useState<number | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const [activity, setActivity] = useState<TerminalActivity>('idle');
  const forceReconnectRef = useRef<(() => void) | null>(null);

  const handleConnectionChange = useCallback((state: ConnectionState) => {
    setConnectionState(state);
  }, []);

  const handleDisconnectedAtChange = useCallback((ts: number | null) => {
    setDisconnectedAt(ts);
  }, []);

  const handleForceReconnectReady = useCallback((fn: () => void) => {
    forceReconnectRef.current = fn;
  }, []);

  const handleReconnect = useCallback(() => {
    forceReconnectRef.current?.();
    haptic.tap();
  }, [haptic]);

  const handleConnect = useCallback(() => {
    haptic.success();
  }, [haptic]);

  const handleDisconnect = useCallback(() => {
    haptic.error();
  }, [haptic]);

  const handleActivityChange = useCallback((a: TerminalActivity) => {
    setActivity(a);
  }, []);

  return (
    <div style={{
      display: 'flex',
      flexDirection: 'column',
      height: 'var(--tg-viewport-height, 100dvh)',
      width: '100%',
      overflow: 'hidden',
    }}>
      <StatusBar
        connectionState={connectionState}
        disconnectedAt={disconnectedAt}
        onReconnect={handleReconnect}
        activity={activity}
      />
      <AuthBanner hapticTap={haptic.tap} />
      <HtmlTerminal
        onConnectionChange={handleConnectionChange}
        onDisconnectedAtChange={handleDisconnectedAtChange}
        onForceReconnectReady={handleForceReconnectReady}
        onHapticConnect={handleConnect}
        onHapticDisconnect={handleDisconnect}
        onActivityChange={handleActivityChange}
      />
      <QuickReply hapticTap={haptic.tap} />
      <StopButton activity={activity} hapticTap={haptic.tap} />
      <InputBar hapticTap={haptic.tap} />
      <KeyBar
        hapticTap={haptic.tap}
        hapticSelection={haptic.selection}
        onCommandDrawer={() => setDrawerOpen(true)}
      />
      <CommandDrawer
        open={drawerOpen}
        onClose={() => setDrawerOpen(false)}
        hapticTap={haptic.tap}
      />
    </div>
  );
}
