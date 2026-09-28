import { useEffect, useRef, useState } from 'react';
import { api, ApiError } from '../api';
import type { Activity, Connection, Item, Space } from '../types';

interface State { space: Space | null; items: Item[]; activities: Activity[]; connection: Connection }
export function useTimeline(id: string, onActivity: (activity: Activity) => void) {
  const [state, setState] = useState<State>({ space: null, items: [], activities: [], connection: 'connecting' });
  const [refresh, setRefresh] = useState(0);
  const notify = useRef(onActivity); notify.current = onActivity;
  useEffect(() => {
    setState({ space: null, items: [], activities: [], connection: 'connecting' });
    if (!id) return;
    let stopped = false, socket: WebSocket | undefined, timer: ReturnType<typeof setTimeout>;
    let sequence = 0, hasSnapshot = false, attempts = 0;
    function reconnect() {
      if (stopped) return;
      setState(s => ({ ...s, connection: 'reconnecting' }));
      timer = setTimeout(connect, Math.min(15000, 750 * 2 ** Math.min(attempts++, 4)) + Math.random() * 500);
    }
    async function connect() {
      if (stopped) return;
      // HTTP gives meaningful auth failures; browsers hide failed WS handshakes.
      try { await api<Space>(`/spaces/${id}`); }
      catch (error) {
        if (stopped) return;
        if (error instanceof ApiError && [401, 403, 404].includes(error.status)) {
          setState({ space: null, items: [], activities: [], connection: 'denied' }); return;
        }
        reconnect(); return;
      }
      if (stopped) return;
      const url = new URL(`/api/spaces/${id}/live`, location.href);
      url.protocol = location.protocol === 'https:' ? 'wss:' : 'ws:';
      socket = new WebSocket(url);
      hasSnapshot = false;
      socket.onmessage = ({ data }) => {
        if (stopped) return;
        try {
          const event = JSON.parse(data);
          if (event.type === 'snapshot') {
            sequence = event.sequence; hasSnapshot = true; attempts = 0;
            setState({ space: event.space, items: event.items, activities: [...event.activities].reverse(), connection: 'live' });
          } else if (event.type === 'activity' && hasSnapshot) {
            const activity = event.activity as Activity;
            if (activity.sequence <= sequence) return;
            if (activity.sequence !== sequence + 1) { socket?.close(); return; }
            sequence = activity.sequence;
            setState(s => ({ ...s,
              items: activity.item.deleted ? s.items.filter(i => i.id !== activity.item.id) : [...s.items.filter(i => i.id !== activity.item.id), activity.item],
              activities: [activity, ...s.activities.filter(a => a.id !== activity.id)].slice(0, 100),
            }));
            notify.current(activity);
          }
        } catch { socket?.close(); }
      };
      socket.onclose = reconnect;
      socket.onerror = () => socket?.close();
    }
    connect();
    return () => { stopped = true; clearTimeout(timer); if (socket) { socket.onclose = null; socket.close(); } };
  }, [id, refresh]);
  // Never render a previous space's data while the effect is resetting.
  return { ...(state.space && state.space.id !== id ? { space: null, items: [], activities: [], connection: 'connecting' as Connection } : state), reconnect: () => setRefresh(n => n + 1) };
}
