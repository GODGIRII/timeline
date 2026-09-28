import { useEffect, useState } from 'react';
import { Activity as ActivityIcon, ArrowDown, Check, Plus, Pencil, RotateCcw, Trash2 } from 'lucide-react';
import { api, messageOf } from '../api';
import { relativeTime } from '../dates';
import type { Activity } from '../types';

export function ActivityFeed({ activities, spaceID, full = false }: { activities: Activity[]; spaceID: string; full?: boolean }) {
  const [older, setOlder] = useState<Activity[]>([]), [loading, setLoading] = useState(false), [error, setError] = useState('');
  useEffect(() => { setOlder([]); setError(''); }, [spaceID]);
  const all = Array.from(new Map([...older, ...activities].map(a => [a.id, a])).values()).sort((a, b) => b.sequence - a.sequence);
  const shown = full ? all : all.slice(0, 8);
  const first = all.length ? Math.min(...all.map(a => a.sequence)) : 0;
  async function loadOlder() {
    setLoading(true); setError('');
    try { const result = await api<{ activities: Activity[] }>(`/spaces/${spaceID}/activities?after=${Math.max(0, first - 51)}&limit=50`); setOlder(old => [...old, ...result.activities]); }
    catch (error) { setError(messageOf(error)); } finally { setLoading(false); }
  }
  return <div className={`activity-feed ${full ? 'full-feed' : ''}`}>
    {!all.length ? <div className="activity-empty"><ActivityIcon size={25} /><h3>A quiet beginning.</h3><p>As your team adds and updates items, you’ll see the story unfold here.</p></div> : <ol>{shown.map(activity => {
      const Icon = activity.kind === 'item.created' ? Plus : activity.kind === 'item.completed' ? Check : activity.kind === 'item.deleted' ? Trash2 : activity.kind === 'item.reopened' ? RotateCcw : Pencil;
      const verb = activity.kind === 'item.created' ? 'added' : activity.kind === 'item.completed' ? 'completed' : activity.kind === 'item.deleted' ? 'removed' : activity.kind === 'item.reopened' ? 'reopened' : 'updated';
      return <li key={activity.id}><span className={`activity-symbol ${activity.kind === 'item.completed' ? 'completed' : ''}`}><Icon size={13} /></span><div><p><strong>{activity.actor.display_name}</strong> {verb} <span className="activity-item-name">{activity.item.title}</span></p><time dateTime={activity.created_at} title={new Date(activity.created_at).toLocaleString()}>{relativeTime(activity.created_at)}</time>{full && activity.changed_fields.length > 0 && <span className="changed-fields">{activity.changed_fields.join(' · ')}</span>}</div></li>;
    })}</ol>}
    {error && <p className="error-note" role="alert">{error}</p>}
    {full && first > 1 && <button className="button secondary" onClick={loadOlder} disabled={loading}><ArrowDown size={15} />{loading ? 'Loading…' : 'Earlier activity'}</button>}
  </div>;
}
