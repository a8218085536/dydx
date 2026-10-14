// SPDX-License-Identifier: Apache-2.0
import { useCallback, useEffect, useRef, useState } from 'react';
import { EV, track } from '../analytics';
import type {
  ConnectionStatus,
  OmsExecutionEvent,
  OmsOrderEvent,
  WebSocketMessage,
} from '../types/market';
import { MessageConflator } from '../utils/conflation';

interface UseWebSocketOptions {
  marketId: number;
  interval: string;
  token: string | null;
  onMessage: (message: WebSocketMessage) => void;
  onOrderEvent?: (event: OmsOrderEvent) => void;
  onExecutionEvent?: (event: OmsExecutionEvent) => void;
  onReconnecting?: () => void;
  onReconnected?: () => void;
}

const MAX_RECONNECT_ATTEMPTS = 10;
const INITIAL_RECONNECT_DELAY = 1000;
const MAX_RECONNECT_DELAY = 30000;
const PING_INTERVAL = 30000;
const HIDDEN_FLUSH_INTERVAL = 250;

let messageCount = 0;
let lastMessageLogTime = 0;

export function wsAuthProtocols(token: string | null): string[] | undefined {
  return token && /^[!#$%&'*+\-.^_`|~0-9A-Za-z]+$/.test(token)
    ? ['bearer', token]
    : undefined;
}

function getWebSocketUrl(): string {
  const protocol = window.location.protocol === 'https:' ? 'wss' : 'ws';
  return `${protocol}://${window.location.host}/ws/v1`;
}

export function useWebSocket({
  marketId,
  interval,
  token,
  onMessage,
  onOrderEvent,
  onExecutionEvent,
  onReconnecting,
  onReconnected,
}: UseWebSocketOptions) {
  const [status, setStatus] = useState<ConnectionStatus>('disconnected');
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectAttemptRef = useRef(0);
  const reconnectTimeoutRef = useRef<number | null>(null);
  const pingIntervalRef = useRef<number | null>(null);
  const shouldReconnectRef = useRef(true);
  const isReconnectRef = useRef(false);
  const marketIdRef = useRef(marketId);
  const intervalRef = useRef(interval);
  const tokenRef = useRef(token);

  const onMessageRef = useRef(onMessage);
  const onOrderEventRef = useRef(onOrderEvent);
  const onExecutionEventRef = useRef(onExecutionEvent);
  const onReconnectingRef = useRef(onReconnecting);
  const onReconnectedRef = useRef(onReconnected);
  onMessageRef.current = onMessage;
  onOrderEventRef.current = onOrderEvent;
  onExecutionEventRef.current = onExecutionEvent;
  onReconnectingRef.current = onReconnecting;
  onReconnectedRef.current = onReconnected;

  const conflatorRef = useRef<MessageConflator | null>(null);
  if (conflatorRef.current === null) conflatorRef.current = new MessageConflator();
  const flushRafRef = useRef<number | null>(null);
  const flushTimerRef = useRef<number | null>(null);

  const clearTimers = useCallback(() => {
    if (reconnectTimeoutRef.current !== null) {
      clearTimeout(reconnectTimeoutRef.current);
      reconnectTimeoutRef.current = null;
    }
    if (pingIntervalRef.current !== null) {
      clearInterval(pingIntervalRef.current);
      pingIntervalRef.current = null;
    }
  }, []);

  const cancelFlush = useCallback(() => {
    if (flushRafRef.current !== null) {
      cancelAnimationFrame(flushRafRef.current);
      flushRafRef.current = null;
    }
    if (flushTimerRef.current !== null) {
      clearTimeout(flushTimerRef.current);
      flushTimerRef.current = null;
    }
  }, []);

  const subscription = useCallback(() => ({
    op: 'subscribe',
    channels: tokenRef.current
      ? ['orderbook', 'trades', 'candles', 'orders', 'executions']
      : ['orderbook', 'trades', 'candles'],
    marketId: marketIdRef.current,
    interval: intervalRef.current,
  }), []);

  const sendSubscription = useCallback((ws: WebSocket | null = wsRef.current) => {
    if (ws?.readyState === WebSocket.OPEN) ws.send(JSON.stringify(subscription()));
  }, [subscription]);

  const flushMessages = useCallback(() => {
    cancelFlush();
    const { messages, refreshMarketIds } = conflatorRef.current!.drain();
    messages.forEach(message => onMessageRef.current(message));
    if (refreshMarketIds.length > 0) sendSubscription();
  }, [cancelFlush, sendSubscription]);

  const scheduleFlush = useCallback(() => {
    if (flushRafRef.current !== null || flushTimerRef.current !== null) return;
    flushRafRef.current = requestAnimationFrame(() => {
      flushRafRef.current = null;
      flushMessages();
    });
    flushTimerRef.current = window.setTimeout(() => {
      flushTimerRef.current = null;
      flushMessages();
    }, HIDDEN_FLUSH_INTERVAL);
  }, [flushMessages]);

  const disconnectSocket = useCallback(() => {
    clearTimers();
    cancelFlush();
    conflatorRef.current!.clear();
    const ws = wsRef.current;
    wsRef.current = null;
    if (ws) {
      ws.onclose = null;
      ws.close();
    }
  }, [cancelFlush, clearTimers]);

  const connect = useCallback(() => {
    const existing = wsRef.current;
    if (existing?.readyState === WebSocket.OPEN || existing?.readyState === WebSocket.CONNECTING) return;
    if (isReconnectRef.current) onReconnectingRef.current?.();
    clearTimers();
    cancelFlush();
    conflatorRef.current!.clear();
    setStatus('connecting');

    try {
      const ws = new WebSocket(getWebSocketUrl(), wsAuthProtocols(tokenRef.current));
      wsRef.current = ws;
      ws.onopen = () => {
        setStatus('connected');
        if (isReconnectRef.current) {
          track(EV.socket_reconnected, { socket: 'unified', attempts: reconnectAttemptRef.current });
        }
        reconnectAttemptRef.current = 0;
        sendSubscription(ws);
        onReconnectedRef.current?.();
        isReconnectRef.current = true;
        pingIntervalRef.current = window.setInterval(() => {
          if (ws.readyState === WebSocket.OPEN) ws.send(JSON.stringify({ op: 'ping' }));
        }, PING_INTERVAL);
      };

      ws.onmessage = event => {
        try {
          const message = JSON.parse(event.data) as Record<string, unknown>;
          if (!('type' in message) && 'omsOrderId' in message) {
            onOrderEventRef.current?.(message as unknown as OmsOrderEvent);
            return;
          }
          if (message.type === 'EXECUTION_HISTORY' || message.type === 'EXECUTION_REPORT') {
            onExecutionEventRef.current?.(message as unknown as OmsExecutionEvent);
            return;
          }
          messageCount++;
          const now = Date.now();
          if (now - lastMessageLogTime >= 5000) {
            const stats = conflatorRef.current!.stats;
            console.log(
              `[WS] +${messageCount} msgs in last 5s | conflation totals:`,
              `delivered ${stats.delivered}, coalesced ${stats.coalesced},`,
              `droppedDeltas ${stats.droppedBookDeltas}, refreshes ${stats.bookRefreshes}`,
            );
            lastMessageLogTime = now;
            messageCount = 0;
          }
          conflatorRef.current!.push(message as unknown as WebSocketMessage);
          scheduleFlush();
        } catch (error) {
          console.error('[WS] Failed to parse message:', error);
        }
      };

      ws.onclose = event => {
        if (wsRef.current !== ws) return;
        wsRef.current = null;
        setStatus('disconnected');
        track(EV.socket_disconnected, {
          socket: 'unified', code: event.code, clean: event.wasClean,
          attempts: reconnectAttemptRef.current,
        });
        clearTimers();
        if (!shouldReconnectRef.current) return;
        if (reconnectAttemptRef.current >= MAX_RECONNECT_ATTEMPTS) {
          setStatus('error');
          return;
        }
        const delay = Math.min(
          INITIAL_RECONNECT_DELAY * Math.pow(1.5, reconnectAttemptRef.current),
          MAX_RECONNECT_DELAY,
        );
        reconnectAttemptRef.current++;
        reconnectTimeoutRef.current = window.setTimeout(connect, delay);
      };
      ws.onerror = () => setStatus('error');
    } catch (error) {
      console.error('[WS] Failed to create WebSocket:', error);
      setStatus('error');
    }
  }, [cancelFlush, clearTimers, scheduleFlush, sendSubscription]);

  const disconnect = useCallback(() => {
    shouldReconnectRef.current = false;
    disconnectSocket();
    setStatus('disconnected');
  }, [disconnectSocket]);

  useEffect(() => {
    tokenRef.current = token;
    shouldReconnectRef.current = true;
    reconnectAttemptRef.current = 0;
    disconnectSocket();
    connect();
    return disconnect;
  }, [token, connect, disconnect, disconnectSocket]);

  useEffect(() => {
    marketIdRef.current = marketId;
    intervalRef.current = interval;
    sendSubscription();
  }, [marketId, interval, sendSubscription]);

  useEffect(() => {
    const onVisible = () => {
      if (document.visibilityState !== 'visible') return;
      if (wsRef.current?.readyState === WebSocket.OPEN) sendSubscription();
      else {
        reconnectAttemptRef.current = 0;
        connect();
      }
    };
    document.addEventListener('visibilitychange', onVisible);
    return () => document.removeEventListener('visibilitychange', onVisible);
  }, [connect, sendSubscription]);

  const requestRefresh = useCallback(() => sendSubscription(), [sendSubscription]);
  const forceReconnect = useCallback(() => {
    isReconnectRef.current = true;
    reconnectAttemptRef.current = 0;
    shouldReconnectRef.current = true;
    disconnectSocket();
    connect();
  }, [connect, disconnectSocket]);

  return { status, reconnect: connect, disconnect, forceReconnect, requestRefresh };
}
