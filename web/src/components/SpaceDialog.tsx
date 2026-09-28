import { useState, type FormEvent } from 'react';
import { ArrowRight, KeyRound, Plus, Clock3 } from 'lucide-react';
import { api, messageOf } from '../api';
import type { Space } from '../types';
import { Modal } from './Modal';

export function SpaceDialog({ mode, onClose, onSpace }: { mode: 'create' | 'join'; onClose: () => void; onSpace: (space: Space) => void }) {
  const [tab, setTab] = useState(mode), [busy, setBusy] = useState(false), [error, setError] = useState(''), [pending, setPending] = useState(false);
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); setBusy(true); setError(''); const data = new FormData(event.currentTarget);
    try {
      if (tab === 'create') { onSpace(await api<Space>('/spaces', 'POST', { name: data.get('value') })); onClose(); }
      else {
        const result = await api<{ status: string; space?: Space }>('/join', 'POST', { key: String(data.get('value')).trim() });
        if (result.space) { onSpace(result.space); onClose(); } else setPending(true);
      }
    } catch (error) { setError(messageOf(error)); } finally { setBusy(false); }
  }
  return <Modal title="A space to move forward." subtitle="Keep a project, a team, or a little part of life in sync." onClose={onClose}>
    {pending ? <div className="pending-state"><span className="empty-symbol"><Clock3 size={26} /></span><h3>You’re on the list.</h3><p>Your request is waiting for the space owner’s approval. The timeline will appear in your sidebar once you’re approved.</p><button className="button primary" onClick={onClose}>Got it</button></div> : <><div className="segmented"><button className={tab === 'create' ? 'selected' : ''} onClick={() => { setTab('create'); setError(''); }}><Plus size={16} /> Create a space</button><button className={tab === 'join' ? 'selected' : ''} onClick={() => { setTab('join'); setError(''); }}><KeyRound size={16} /> Join a space</button></div><form key={tab} className="form-stack space-form" onSubmit={submit}><label>{tab === 'create' ? 'Space name' : 'Invitation key'}<input name="value" autoFocus required maxLength={tab === 'create' ? 100 : 128} placeholder={tab === 'create' ? 'e.g. Product launch, Home, Side projects' : 'Paste the key shared with you'} /></label><p className="muted small">{tab === 'create' ? 'You’ll be the admin. Invite people and choose who can edit.' : 'The space owner will approve your request before you can view the timeline.'}</p>{error && <p className="error-note" role="alert">{error}</p>}<button className="button primary button-full" disabled={busy}>{busy ? 'One moment…' : tab === 'create' ? 'Create space' : 'Request to join'}<ArrowRight size={16} /></button></form></>}
  </Modal>;
}
