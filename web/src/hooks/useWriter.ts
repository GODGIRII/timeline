import { useState } from 'react';
import { api, ApiError } from '../api';
import type { WriteCommand } from '../types';

// Persist uncertain writes per account so retries reuse the original operation ID,
// even after refreshing. Only one unresolved item write is allowed at a time.
export function useWriter(accountID: string) {
  const key = `timeline-pending-${accountID}`;
  const [pending, setPending] = useState<WriteCommand | null>(() => {
    try { return JSON.parse(sessionStorage.getItem(key) || 'null'); } catch { return null; }
  });
  const [busy, setBusy] = useState(false);
  async function write(command: WriteCommand) {
    setBusy(true); setPending(command);
    try { sessionStorage.setItem(key, JSON.stringify(command)); } catch { /* Private storage can be unavailable. */ }
    try {
      const result = await api(command.path, command.method, command.body);
      setPending(null); sessionStorage.removeItem(key); return result;
    } catch (error) {
      if (error instanceof ApiError && error.status > 0 && error.status < 500) {
        setPending(null); sessionStorage.removeItem(key);
      }
      throw error;
    } finally { setBusy(false); }
  }
  return { pending, busy, write };
}
