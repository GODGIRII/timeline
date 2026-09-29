import { useState, type FormEvent } from 'react';
import { ArrowRight, Check, Layers3, Sparkles } from 'lucide-react';
import { api, messageOf } from '../api';
import type { Account } from '../types';

export function Auth({ onLogin }: { onLogin: (account: Account) => void }) {
  const [register, setRegister] = useState(false), [busy, setBusy] = useState(false), [error, setError] = useState('');
  async function submit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault(); const fields = new FormData(event.currentTarget); setBusy(true); setError('');
    try {
      const body = { username: fields.get('username'), password: fields.get('password'), ...(register ? { display_name: fields.get('display_name') } : {}) };
      onLogin(await api<Account>(`/auth/${register ? 'register' : 'login'}`, 'POST', body));
    } catch (error) { setError(messageOf(error)); } finally { setBusy(false); }
  }
  return <main className="auth-page">
    <section className="auth-story"><a href="/" className="brand"><span className="brand-icon"><Layers3 size={22} /></span>sequence<span className="brand-dot">.</span></a>
      <div className="auth-copy"><span className="eyebrow"><Sparkles size={15} /> A LITTLE CLARITY, EVERY DAY</span><h1>Good things<br />take a little<br /><em>planning.</em></h1><p>A calmer place for your tasks, your people,<br className="desktop-only" /> and everything coming next.</p>
        <div className="auth-preview" aria-hidden="true"><div className="preview-heading"><span>One shared view.</span><span className="live-label"><i /> Together</span></div><div className="preview-line"><span className="preview-check"><Check size={14} /></span><span>A plan everyone can follow</span><span className="priority high">High</span></div><div className="preview-line"><span className="preview-check"><Check size={14} /></span><span>Less chasing. More doing.</span><span className="priority medium">Medium</span></div><div className="preview-foot"><span className="avatar-stack"><i>A</i><i>M</i><i>J</i></span><span>Better, together.</span></div></div>
      </div><p className="auth-bottom">A little structure. A lot more possibility.</p>
    </section>
    <section className="auth-form-side"><div className="auth-form-wrap"><span className="eyebrow">YOUR NEXT CHAPTER</span><h2>{register ? 'Make space for more.' : 'Welcome back.'}</h2><p>{register ? 'Create your account and start a shared timeline.' : 'Your plans, people, and priorities are right here.'}</p>
      <form onSubmit={submit} className="form-stack" key={String(register)}>
        {register && <label>Your name<input name="display_name" placeholder="How should we call you?" autoComplete="name" required maxLength={100} /></label>}
        <label>Username<input name="username" placeholder="e.g. alex_morgan" autoComplete="username" autoCapitalize="none" required minLength={3} maxLength={32} pattern="[A-Za-z0-9_]+" /></label>
        <label>Password<input name="password" type="password" placeholder={register ? 'At least 12 characters' : 'Enter your password'} autoComplete={register ? 'new-password' : 'current-password'} required minLength={register ? 12 : undefined} maxLength={256} /></label>
        {error && <div className="error-note" role="alert">{error}</div>}
        <button className="button primary button-full" disabled={busy}>{busy ? 'One moment…' : register ? 'Create account' : 'Sign in'}<ArrowRight size={17} /></button>
      </form><p className="auth-switch">{register ? 'Already have an account?' : 'New around here?'} <button onClick={() => { setRegister(!register); setError(''); }}>{register ? 'Sign in' : 'Create an account'}</button></p><div className="auth-note"><span className="tiny-dot" /> Your plans stay in the spaces you choose to share.</div>
    </div></section>
  </main>;
}
