// SPDX-License-Identifier: Apache-2.0
// @vitest-environment jsdom
import { act, renderHook } from '@testing-library/react';
import { beforeEach, describe, expect, it, vi } from 'vitest';
import { useWebSocket, wsAuthProtocols } from './useWebSocket';
import type { OmsExecutionEvent, OmsOrderEvent, WebSocketMessage } from '../types/market';

class MockWebSocket {
  static CONNECTING = 0;
  static OPEN = 1;
  static CLOSING = 2;
  static CLOSED = 3;
  static instances: MockWebSocket[] = [];

  readonly sent: string[] = [];
  readyState = MockWebSocket.CONNECTING;
  onopen: (() => void) | null = null;
  onclose: ((event: CloseEvent) => void) | null = null;
  onerror: (() => void) | null = null;
  onmessage: ((event: MessageEvent<string>) => void) | null = null;
  closed = false;

  constructor(readonly url: string, readonly protocols?: string | string[]) {
    MockWebSocket.instances.push(this);
  }

  open() {
    this.readyState = MockWebSocket.OPEN;
    this.onopen?.();
  }

  message(value: unknown) {
    this.onmessage?.({ data: JSON.stringify(value) } as MessageEvent<string>);
  }

  send(value: string) {
    this.sent.push(value);
  }

  close() {
    this.closed = true;
    this.readyState = MockWebSocket.CLOSED;
  }
}

describe('useWebSocket', () => {
  const rafCallbacks: FrameRequestCallback[] = [];

  beforeEach(() => {
    MockWebSocket.instances = [];
    rafCallbacks.length = 0;
    globalThis.WebSocket = MockWebSocket as unknown as typeof WebSocket;
    globalThis.requestAnimationFrame = vi.fn((callback: FrameRequestCallback) => {
      rafCallbacks.push(callback);
      return rafCallbacks.length;
    });
    globalThis.cancelAnimationFrame = vi.fn();
  });

  it('reconnects between anonymous and authenticated subscriptions', () => {
    const callbacks = {
      onMessage: vi.fn<(message: WebSocketMessage) => void>(),
      onOrderEvent: vi.fn<(event: OmsOrderEvent) => void>(),
      onExecutionEvent: vi.fn<(event: OmsExecutionEvent) => void>(),
    };
    const { rerender, unmount } = renderHook(
      ({ token }) => useWebSocket({
        marketId: 1,
        interval: '5m',
        token,
        ...callbacks,
      }),
      { initialProps: { token: null as string | null } },
    );

    expect(MockWebSocket.instances).toHaveLength(1);
    expect(MockWebSocket.instances[0].url).toMatch(/\/ws\/v1$/);
    expect(MockWebSocket.instances[0].protocols).toBeUndefined();
    act(() => MockWebSocket.instances[0].open());
    expect(JSON.parse(MockWebSocket.instances[0].sent[0])).toEqual({
      op: 'subscribe',
      channels: ['orderbook', 'trades', 'candles'],
      marketId: 1,
      interval: '5m',
    });

    rerender({ token: 'session-token-1' });
    expect(MockWebSocket.instances[0].closed).toBe(true);
    expect(MockWebSocket.instances).toHaveLength(2);
    expect(MockWebSocket.instances[1].protocols).toEqual(['bearer', 'session-token-1']);
    act(() => MockWebSocket.instances[1].open());
    expect(JSON.parse(MockWebSocket.instances[1].sent[0]).channels).toEqual([
      'orderbook', 'trades', 'candles', 'orders', 'executions',
    ]);

    rerender({ token: null });
    expect(MockWebSocket.instances[1].closed).toBe(true);
    expect(MockWebSocket.instances).toHaveLength(3);
    expect(MockWebSocket.instances[2].protocols).toBeUndefined();
    unmount();
  });

  it('routes public frames through conflation and private frames immediately', () => {
    const onMessage = vi.fn<(message: WebSocketMessage) => void>();
    const onOrderEvent = vi.fn<(event: OmsOrderEvent) => void>();
    const onExecutionEvent = vi.fn<(event: OmsExecutionEvent) => void>();
    renderHook(() => useWebSocket({
      marketId: 1,
      interval: '1m',
      token: 'session-token-1',
      onMessage,
      onOrderEvent,
      onExecutionEvent,
    }));
    const socket = MockWebSocket.instances[0];

    act(() => socket.message({ omsOrderId: '42', userId: 7, status: 'NEW' }));
    act(() => socket.message({
      type: 'EXECUTION_REPORT',
      execution: { tradeId: '8', userId: 7, marketId: 1 },
    }));
    act(() => socket.message({ type: 'TRADES_BATCH', marketId: 1, trades: [] }));

    expect(onOrderEvent).toHaveBeenCalledTimes(1);
    expect(onExecutionEvent).toHaveBeenCalledTimes(1);
    expect(onMessage).not.toHaveBeenCalled();
    act(() => rafCallbacks.splice(0).forEach(callback => callback(1)));
    expect(onMessage).toHaveBeenCalledWith(expect.objectContaining({ type: 'TRADES_BATCH' }));
  });
});

describe('wsAuthProtocols', () => {
  it('uses token-safe session tokens as bearer subprotocols', () => {
    expect(wsAuthProtocols('a1b2c3-XYZ.~_')).toEqual(['bearer', 'a1b2c3-XYZ.~_']);
    expect(wsAuthProtocols('dev:1')).toBeUndefined();
  });
});
