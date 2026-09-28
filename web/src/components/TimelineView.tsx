import { useState } from 'react';
import { CalendarDays, Check, ChevronLeft, ChevronRight, Clock3, Flag, ListTodo, Plus } from 'lucide-react';
import { dateKey, dayLabel, itemDate, overdue, timeLabel } from '../dates';
import type { Item } from '../types';

export function TimelineView({ items, canEdit, onEdit, onToggle, onCreate, calendar }: {
  items: Item[]; canEdit: boolean; onEdit: (item: Item) => void; onToggle: (item: Item) => void; onCreate: () => void; calendar: boolean;
}) {
  const [month, setMonth] = useState(() => new Date(new Date().getFullYear(), new Date().getMonth(), 1));
  if (calendar) {
    const start = new Date(month); start.setDate(1 - ((month.getDay() + 6) % 7));
    const days = Array.from({ length: 42 }, (_, n) => { const day = new Date(start); day.setDate(start.getDate() + n); return day; });
    return <div className="calendar-wrap"><div className="calendar-heading"><h3>{month.toLocaleDateString(undefined, { month: 'long', year: 'numeric' })}</h3><div className="calendar-controls"><button className="text-button" onClick={() => setMonth(new Date(new Date().getFullYear(), new Date().getMonth(), 1))}>Today</button><button className="icon-button" aria-label="Previous month" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() - 1, 1))}><ChevronLeft size={18} /></button><button className="icon-button" aria-label="Next month" onClick={() => setMonth(new Date(month.getFullYear(), month.getMonth() + 1, 1))}><ChevronRight size={18} /></button></div></div><div className="calendar-scroll"><div className="calendar-grid">{['Mon', 'Tue', 'Wed', 'Thu', 'Fri', 'Sat', 'Sun'].map(day => <div key={day} className="weekday">{day}</div>)}{days.map(day => <div key={dateKey(day)} className={`calendar-day ${day.getMonth() !== month.getMonth() ? 'other-month' : ''} ${dateKey(day) === dateKey() ? 'today' : ''}`}><span className="day-number">{day.getDate()}</span>{items.filter(item => itemDate(item) === dateKey(day)).map(item => <button key={item.id} className={`calendar-item ${item.priority} ${item.status === 'done' ? 'is-done' : ''}`} onClick={() => onEdit(item)} title={item.title}><i />{item.title}</button>)}</div>)}</div></div></div>;
  }
  const groups = new Map<string, Item[]>();
  for (const item of items) { const key = itemDate(item); groups.set(key, [...(groups.get(key) || []), item]); }
  if (!items.length) return <div className="timeline-empty"><span className="empty-symbol"><ListTodo size={28} strokeWidth={1.5} /></span><h3>A little space for what’s next.</h3><p>No items match this view. {canEdit ? 'Add a task or event, or try a different filter.' : 'New plans will appear here as your team adds them.'}</p>{canEdit && <button className="button secondary" onClick={onCreate}><Plus size={16} /> Add an item</button>}</div>;
  return <div className="timeline-groups">{[...groups].map(([date, group]) => <section className="day-group" key={date}><div className="day-heading"><span className={`day-dot ${date === dateKey() ? 'is-today' : ''}`} /><h3>{dayLabel(date)}</h3><span>{group.length} {group.length === 1 ? 'item' : 'items'}</span><span className="day-rule" /></div><div className="day-items">{group.map(item => <article className={`item-card ${item.status === 'done' ? 'is-done' : ''}`} key={item.id} data-testid="item-card">
      <button className={`item-check ${item.status === 'done' ? 'checked' : ''}`} aria-label={`${item.status === 'done' ? 'Reopen' : 'Complete'} ${item.title}`} disabled={!canEdit} onClick={() => onToggle(item)}>{item.status === 'done' && <Check size={15} />}</button>
      <button className="item-content" onClick={() => onEdit(item)}><div className="item-title-line"><h4>{item.title}</h4><span className={`priority ${item.priority}`}><i />{item.priority}</span></div>{item.description && <p>{item.description}</p>}<div className="item-meta"><span>{item.type === 'event' ? <CalendarDays size={13} /> : <ListTodo size={13} />}{item.type === 'event' ? 'Event' : 'Task'}</span><span><Clock3 size={13} />{timeLabel(item)}</span>{overdue(item) && <span className="overdue"><Flag size={12} />Overdue</span>}{item.status === 'done' && <span className="done-label">Completed</span>}</div></button><ChevronRight size={17} className="item-chevron" />
    </article>)}</div></section>)}</div>;
}
