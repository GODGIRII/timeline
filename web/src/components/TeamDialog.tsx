import { useEffect, useState } from 'react';
import { Check, Copy, KeyRound, RefreshCw, ShieldCheck, UserPlus, X } from 'lucide-react';
import { api, messageOf } from '../api';
import type { MemberData, Role, Space } from '../types';
import { Modal } from './Modal';

export function TeamDialog({ space, onClose }: { space: Space; onClose: () => void }) {
  const [data, setData] = useState<MemberData | null>(null), [key, setKey] = useState(space.key || ''), [error, setError] = useState('');
  const [busy, setBusy] = useState(''), [copied, setCopied] = useState(false), [rotate, setRotate] = useState(false);
  async function load() { setData(await api<MemberData>(`/spaces/${space.id}/members`)); }
  useEffect(() => {
    let active = true;
    const fetchMembers = async () => { try { const result = await api<MemberData>(`/spaces/${space.id}/members`); if (active) setData(result); } catch (error) { if (active) setError(messageOf(error)); } };
    fetchMembers(); const timer = setInterval(fetchMembers, 10000); return () => { active = false; clearInterval(timer); };
  }, [space.id]);
  async function change(id: string, role: Role | 'revoked' | 'rejected') {
    setBusy(id); setError('');
    try { await api(`/spaces/${space.id}/members/${id}`, 'POST', { role }); await load(); } catch (error) { setError(messageOf(error)); } finally { setBusy(''); }
  }
  async function rotateKey() {
    setBusy('key'); setError('');
    try { const result = await api<{ key: string }>(`/spaces/${space.id}/rotate-key`, 'POST', {}); setKey(result.key); setRotate(false); setCopied(false); } catch (error) { setError(messageOf(error)); } finally { setBusy(''); }
  }
  const requests = data?.requests.filter(r => r.status === 'pending') || [];
  return <Modal title="Better with your people." subtitle={`Manage access to ${space.name}.`} onClose={onClose} wide>
    <div className="invite-box"><div className="invite-title"><KeyRound size={18} /><strong>Invite someone in</strong></div><p>Share this key. You’ll approve each person before they get access.</p><div className="copy-row"><input aria-label="Invitation key" value={key} readOnly /><button className="button secondary" onClick={async () => { try { await navigator.clipboard.writeText(key); setCopied(true); } catch { setError('Copy isn’t available. Select and copy the key manually.'); } }}>{copied ? <Check size={16} /> : <Copy size={16} />}{copied ? 'Copied' : 'Copy'}</button></div>{rotate ? <div className="rotate-confirm"><span>Replace the key? Existing members keep access.</span><button className="text-button" disabled={!!busy} onClick={rotateKey}>Replace key</button><button className="text-button muted" onClick={() => setRotate(false)}>Cancel</button></div> : <button className="text-button muted" onClick={() => setRotate(true)}><RefreshCw size={12} /> Generate a new key</button>}</div>
    {error && <p className="error-note" role="alert">{error}</p>}
    {requests.length > 0 && <section className="team-section"><h3><UserPlus size={17} /> Waiting to join <span className="count">{requests.length}</span></h3>{requests.map(({ account }) => <div className="member-row" key={account.id}><span className="avatar">{account.display_name.slice(0, 1)}</span><div className="member-name"><strong>{account.display_name}</strong><span>@{account.username}</span></div><div className="member-actions"><button className="button secondary small-button" disabled={!!busy} onClick={() => change(account.id, 'viewer')}>Allow viewing</button><button className="button primary small-button" disabled={!!busy} onClick={() => change(account.id, 'editor')}>Allow editing</button><button className="icon-button" aria-label={`Reject ${account.display_name}`} disabled={!!busy} onClick={() => change(account.id, 'rejected')}><X size={16} /></button></div></div>)}</section>}
    <section className="team-section"><h3><ShieldCheck size={17} /> People with access <span className="count">{data?.members.length || 0}</span></h3>{!data && <p className="muted">Loading people…</p>}{data?.members.map(({ account, role }) => <div className="member-row" key={account.id}><span className="avatar">{account.display_name.slice(0, 1)}</span><div className="member-name"><strong>{account.display_name}</strong><span>@{account.username}</span></div>{role === 'owner' ? <span className="role-badge">Admin</span> : <div className="member-actions"><select aria-label={`Permission for ${account.display_name}`} value={role} disabled={!!busy} onChange={e => change(account.id, e.target.value as Role)}><option value="viewer">Can view</option><option value="editor">Can edit</option></select><button className="icon-button danger-text" title="Remove access" aria-label={`Remove ${account.display_name}`} disabled={!!busy} onClick={() => change(account.id, 'revoked')}><X size={16} /></button></div>}</div>)}</section>
    <p className="muted small team-foot">Editors can create and update all items. Viewers can follow the timeline and activity.</p>
  </Modal>;
}
