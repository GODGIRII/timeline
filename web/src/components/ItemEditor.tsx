import { useState, type FormEvent } from 'react';
import { CalendarDays, Check, ListTodo, RefreshCw, Trash2 } from 'lucide-react';
import { api, ApiError, messageOf } from '../api';
import { dateKey } from '../dates';
import type { Item, ItemFields, WriteCommand } from '../types';
import { Modal } from './Modal';

function initial(item?: Item): ItemFields { return item ? { type: item.type, title: item.title, description: item.description, deadline: item.deadline, priority: item.priority, status: item.status } : { type: 'task', title: '', description: '', deadline: { date: dateKey() }, priority: 'medium', status: 'open' }; }
function dateInput(item?: Item) { return item?.deadline.at ? dateKey(new Date(item.deadline.at)) : item?.deadline.date || dateKey(); }
function timeInput(item?: Item) { if (!item?.deadline.at) return ''; const d = new Date(item.deadline.at); return `${String(d.getHours()).padStart(2, '0')}:${String(d.getMinutes()).padStart(2, '0')}`; }

export function ItemEditor({ spaceID, item: original, canEdit, blocked, busy, write, onClose }: {
  spaceID: string; item?: Item; canEdit: boolean; blocked: boolean; busy: boolean;
  write: (command: WriteCommand) => Promise<unknown>; onClose: () => void;
}) {
  const [item, setItem] = useState(original), [fields, setFields] = useState(() => initial(original));
  const [date, setDate] = useState(dateInput(original)), [time, setTime] = useState(timeInput(original));
  const [error, setError] = useState(''), [conflict, setConflict] = useState(false), [removing, setRemoving] = useState(false);
  const disabled = !canEdit || blocked || busy;
  async function reload() {
    try {
      const latest = await api<Item>(`/spaces/${spaceID}/items/${item!.id}`);
      setItem(latest); setFields(initial(latest)); setDate(dateInput(latest)); setTime(timeInput(latest)); setError(''); setConflict(false);
    } catch (error) { setError(messageOf(error)); }
  }
  async function submit(event: FormEvent) {
    event.preventDefault(); if (disabled) return; setError('');
    try {
      // Leave an existing instant untouched unless the user actually changes it.
      const deadline = item && date === dateInput(item) && time === timeInput(item) ? item.deadline : time ? { at: new Date(`${date}T${time}`).toISOString() } : { date };
      await write({ path: `/spaces/${spaceID}/items${item ? `/${item.id}` : ''}`, method: item ? 'PUT' : 'POST', body: { ...fields, deadline, operation_id: crypto.randomUUID(), ...(item ? { base_version: item.version } : {}) } });
      onClose();
    } catch (error) { setError(messageOf(error)); setConflict(error instanceof ApiError && error.status === 409); }
  }
  async function remove() {
    if (disabled) return;
    try { await write({ path: `/spaces/${spaceID}/items/${item!.id}`, method: 'DELETE', body: { operation_id: crypto.randomUUID(), base_version: item!.version } }); onClose(); }
    catch (error) { setError(messageOf(error)); setConflict(error instanceof ApiError && error.status === 409); }
  }
  return <Modal title={item ? canEdit ? 'A little change of plans.' : 'The details.' : 'What’s coming next?'} subtitle={item ? 'Keep everyone on the same page.' : 'Give your next task or event a place in the timeline.'} onClose={onClose}>
    <form onSubmit={submit} className="form-stack">
      <fieldset disabled={disabled} className="form-stack fieldset-clean">
        <div className="segmented type-picker" role="group" aria-label="Item type"><button type="button" aria-pressed={fields.type === 'task'} className={fields.type === 'task' ? 'selected' : ''} onClick={() => setFields({ ...fields, type: 'task' })}><ListTodo size={17} /> Task</button><button type="button" aria-pressed={fields.type === 'event'} className={fields.type === 'event' ? 'selected' : ''} onClick={() => setFields({ ...fields, type: 'event' })}><CalendarDays size={17} /> Event</button></div>
        <label>Title<input autoFocus placeholder="What would you like to get done?" value={fields.title} onChange={e => setFields({ ...fields, title: e.target.value })} required maxLength={200} /></label>
        <label>Description <span className="label-hint">optional</span><textarea rows={3} placeholder="A little context goes a long way…" value={fields.description} onChange={e => setFields({ ...fields, description: e.target.value })} maxLength={5000} /></label>
        <div className="form-row"><label>Deadline<input type="date" value={date} onChange={e => setDate(e.target.value)} required /></label><label>Time <span className="label-hint">optional · local</span><input type="time" value={time} onChange={e => setTime(e.target.value)} /></label></div>
        <label>Priority</label><div className="priority-picker" role="group" aria-label="Priority">{(['low', 'medium', 'high'] as const).map(priority => <button type="button" key={priority} className={`${priority} ${fields.priority === priority ? 'chosen' : ''}`} aria-pressed={fields.priority === priority} onClick={() => setFields({ ...fields, priority })}><i />{priority}{fields.priority === priority && <Check size={14} />}</button>)}</div>
        {item && <label>Status<select value={fields.status} onChange={e => setFields({ ...fields, status: e.target.value as 'open' | 'done' })}><option value="open">Open</option><option value="done">Done</option></select></label>}
      </fieldset>
      {!canEdit && <p className="muted small">This timeline is read-only for you right now.</p>}
      {blocked && <p className="error-note">A previous save is awaiting confirmation. Close this dialog and retry it from the connection banner.</p>}
      {error && <div className="error-note" role="alert">{error}{conflict && <button type="button" className="text-button" onClick={reload}><RefreshCw size={14} /> Reload latest version</button>}</div>}
      {removing && <div className="delete-confirm"><p>Remove “{item?.title}”? Its activity history will be kept.</p><button type="button" className="button danger" disabled={disabled} onClick={remove}>Confirm removal</button><button type="button" className="button subtle" onClick={() => setRemoving(false)}>Keep item</button></div>}
      <footer className="modal-actions">{item && canEdit && <button type="button" className="icon-button danger-text" aria-label="Remove item" disabled={disabled} onClick={() => setRemoving(true)}><Trash2 size={18} /></button>}<span className="flex-1" /><button type="button" className="button secondary" onClick={onClose}>{canEdit ? 'Cancel' : 'Close'}</button>{canEdit && <button className="button primary" disabled={disabled || conflict}>{busy ? 'Saving…' : item ? 'Save changes' : `Create ${fields.type}`}<Check size={16} /></button>}</footer>
    </form>
  </Modal>;
}
