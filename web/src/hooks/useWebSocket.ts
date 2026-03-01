'use client';

import { useEffect, useRef } from 'react';
import { useInstanceStore } from '@/store/instances';
import type { RalphEvent, InstanceState } from '@/lib/types';

export function useWebSocket(url: string) {
  const wsRef = useRef<WebSocket | null>(null);
  const reconnectTimerRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const backoffRef = useRef(1000);
  const unmountedRef = useRef(false);

  useEffect(() => {
    unmountedRef.current = false;

    function deriveRestUrl(wsUrl: string): string {
      const parsed = new URL(wsUrl);
      parsed.protocol = parsed.protocol === 'wss:' ? 'https:' : 'http:';
      parsed.pathname = '/api/v1/instances';
      return parsed.toString();
    }

    function connect() {
      if (unmountedRef.current) return;

      const ws = new WebSocket(url);
      wsRef.current = ws;

      ws.onopen = async () => {
        backoffRef.current = 1000;

        // Fetch current instance state from the REST API
        try {
          const restUrl = deriveRestUrl(url);
          const res = await fetch(restUrl);
          if (res.ok) {
            const instances: InstanceState[] = await res.json();
            useInstanceStore.getState().setInstances(instances);
          }
        } catch {
          // Silently ignore fetch errors; events will populate state
        }
      };

      ws.onmessage = (msg) => {
        try {
          const event: RalphEvent = JSON.parse(msg.data);
          useInstanceStore.getState().handleEvent(event);
        } catch {
          // Ignore malformed messages
        }
      };

      ws.onclose = () => {
        if (unmountedRef.current) return;
        scheduleReconnect();
      };

      ws.onerror = () => {
        // onclose will fire after onerror, which triggers reconnect
        ws.close();
      };
    }

    function scheduleReconnect() {
      if (unmountedRef.current) return;

      const delay = backoffRef.current;
      backoffRef.current = Math.min(delay * 2, 30000);

      reconnectTimerRef.current = setTimeout(() => {
        reconnectTimerRef.current = null;
        connect();
      }, delay);
    }

    connect();

    return () => {
      unmountedRef.current = true;

      if (reconnectTimerRef.current !== null) {
        clearTimeout(reconnectTimerRef.current);
        reconnectTimerRef.current = null;
      }

      if (wsRef.current) {
        wsRef.current.close();
        wsRef.current = null;
      }
    };
  }, [url]);
}
