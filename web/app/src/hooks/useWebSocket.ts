import { useCallback, useEffect, useRef, useState } from 'react';

export type ConnectionState = 'connecting' | 'connected' | 'disconnected';

interface UseWebSocketOptions {
  onData: (data: ArrayBuffer) => void;
  onConnect?: () => void;
  onDisconnect?: () => void;
}

/**
 * WebSocket connection to the workspace terminal server.
 *
 * Features:
 * - Exponential backoff reconnection (1s → 2s → 4s → 8s → 15s max)
 * - Binary messages for terminal I/O
 * - JSON text messages for resize control
 * - Force-reconnect support for manual retry
 * - Graceful cleanup on unmount
 */
export function useWebSocket(options: UseWebSocketOptions) {
  const [state, setState] = useState<ConnectionState>('connecting');
  const [disconnectedAt, setDisconnectedAt] = useState<number | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimer = useRef<ReturnType<typeof setTimeout> | null>(null);
  const reconnectDelay = useRef(1000);
  const mountedRef = useRef(true);
  const optionsRef = useRef(options);
  optionsRef.current = options;

  const getWsUrl = useCallback(() => {
    const params = new URLSearchParams(window.location.search);
    const token = params.get('token') || '';
    const instance = params.get('instance') || '';
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    let url = `${proto}//${window.location.host}/ws?token=${encodeURIComponent(token)}`;
    if (instance) url += `&instance=${encodeURIComponent(instance)}`;
    return url;
  }, []);

  const connect = useCallback(() => {
    if (!mountedRef.current) return;

    // Close any existing connection before opening a new one
    if (wsRef.current) {
      wsRef.current.onclose = null;
      wsRef.current.close();
    }

    const url = getWsUrl();
    const ws = new WebSocket(url);
    ws.binaryType = 'arraybuffer';
    wsRef.current = ws;

    setState('connecting');

    ws.onopen = () => {
      if (!mountedRef.current) { ws.close(); return; }
      setState('connected');
      setDisconnectedAt(null);
      reconnectDelay.current = 1000; // Reset backoff
      optionsRef.current.onConnect?.();
    };

    ws.onmessage = (event) => {
      if (event.data instanceof ArrayBuffer) {
        optionsRef.current.onData(event.data);
      }
    };

    ws.onclose = () => {
      if (!mountedRef.current) return;
      setState((prev) => {
        // Only set disconnectedAt on the first transition to disconnected
        if (prev !== 'disconnected') {
          setDisconnectedAt(Date.now());
        }
        return 'disconnected';
      });
      optionsRef.current.onDisconnect?.();
      scheduleReconnect();
    };

    ws.onerror = () => {
      // onclose will fire after onerror, so we just let it handle reconnection
    };
  }, [getWsUrl]);

  const scheduleReconnect = useCallback(() => {
    if (!mountedRef.current) return;
    if (reconnectTimer.current) clearTimeout(reconnectTimer.current);

    reconnectTimer.current = setTimeout(() => {
      if (mountedRef.current) {
        connect();
      }
    }, reconnectDelay.current);

    // Exponential backoff: 1s → 2s → 4s → 8s → 15s (cap)
    reconnectDelay.current = Math.min(reconnectDelay.current * 2, 15000);
  }, [connect]);

  // Force an immediate reconnect attempt (cancels pending scheduled reconnect)
  const forceReconnect = useCallback(() => {
    if (!mountedRef.current) return;
    if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
    reconnectDelay.current = 1000; // Reset backoff on manual retry
    connect();
  }, [connect]);

  // Send binary data (terminal input)
  const send = useCallback((data: string | Uint8Array) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;

    if (typeof data === 'string') {
      const encoder = new TextEncoder();
      ws.send(encoder.encode(data));
    } else {
      ws.send(data);
    }
  }, []);

  // Send resize event as JSON text message
  const sendResize = useCallback((cols: number, rows: number) => {
    const ws = wsRef.current;
    if (!ws || ws.readyState !== WebSocket.OPEN) return;
    ws.send(JSON.stringify({ type: 'resize', cols, rows }));
  }, []);

  // Connect on mount
  useEffect(() => {
    mountedRef.current = true;
    connect();

    return () => {
      mountedRef.current = false;
      if (reconnectTimer.current) clearTimeout(reconnectTimer.current);
      if (wsRef.current) {
        wsRef.current.onclose = null; // Prevent reconnection on unmount
        wsRef.current.close();
      }
    };
  }, [connect]);

  return { state, disconnectedAt, send, sendResize, forceReconnect };
}
