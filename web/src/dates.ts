import type { Item } from './types';

export function dateKey(date = new Date()): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, '0')}-${String(date.getDate()).padStart(2, '0')}`;
}
export function itemDate(item: Item): string { return item.deadline.date || dateKey(new Date(item.deadline.at!)); }
export function dayLabel(key: string): string {
  const today = dateKey();
  const tomorrow = new Date(); tomorrow.setDate(tomorrow.getDate() + 1);
  const date = new Date(`${key}T12:00:00`);
  const label = date.toLocaleDateString(undefined, { month: 'short', day: 'numeric', ...(date.getFullYear() !== new Date().getFullYear() ? { year: 'numeric' } : {}) });
  if (key === today) return `Today · ${label}`;
  if (key === dateKey(tomorrow)) return `Tomorrow · ${label}`;
  return `${date.toLocaleDateString(undefined, { weekday: 'long' })} · ${label}`;
}
export function overdue(item: Item): boolean {
  if (item.status === 'done') return false;
  return item.deadline.date ? item.deadline.date < dateKey() : new Date(item.deadline.at!).getTime() < Date.now();
}
export function timeLabel(item: Item): string {
  return item.deadline.at ? new Date(item.deadline.at).toLocaleTimeString(undefined, { hour: 'numeric', minute: '2-digit' }) : 'Any time';
}
export function relativeTime(value: string): string {
  const minutes = Math.max(0, Math.floor((Date.now() - new Date(value).getTime()) / 60000));
  if (minutes < 1) return 'Just now';
  if (minutes < 60) return `${minutes}m ago`;
  if (minutes < 1440) return `${Math.floor(minutes / 60)}h ago`;
  return new Date(value).toLocaleDateString(undefined, { month: 'short', day: 'numeric' });
}
