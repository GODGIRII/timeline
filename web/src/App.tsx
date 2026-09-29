import { useCallback, useEffect, useRef, useState } from 'react';
import { Activity as ActivityIcon, ArrowDownWideNarrow, ArrowRight, CalendarDays, Check, CheckCheck, ChevronDown, CircleHelp, Clock3, Flag, Layers3, LayoutList, LogOut, Menu, Plus, Search, Settings2, Sparkles, Users, WifiOff, X } from 'lucide-react';
import { api, ApiError, messageOf } from './api';
import { dateKey, itemDate, overdue } from './dates';
import { Auth } from './components/Auth';
import { ItemEditor } from './components/ItemEditor';
import { SpaceDialog } from './components/SpaceDialog';
import { TeamDialog } from './components/TeamDialog';
import { ActivityFeed } from './components/ActivityFeed';
import { TimelineView } from './components/TimelineView';
import { Modal } from './components/Modal';
import { useTimeline } from './hooks/useTimeline';
import { useWriter } from './hooks/useWriter';
import type { Account, Activity, Item, Space } from './types';

export default function App() {
  const [account, setAccount] = useState<Account | null>(null), [loading, setLoading] = useState(true), [error, setError] = useState('');
  const load = useCallback(async () => {
    setError(''); setLoading(true);
    try { setAccount(await api<Account>('/me')); }
    catch (error) { if (!(error instanceof ApiError && error.status === 401)) setError(messageOf(error)); }
    finally { setLoading(false); }
  }, []);
  useEffect(() => { load(); const expired = () => setAccount(null); window.addEventListener('session-expired', expired); return () => window.removeEventListener('session-expired', expired); }, [load]);
  if (loading || error) return <main className="boot-screen"><span className="brand-icon"><Layers3 /></span><h1>sequence.</h1>{error ? <><p role="alert">{error}</p><button className="button primary" onClick={load}>Try again</button></> : <p className="pulse">Getting your plans together…</p>}</main>;
  return account ? <Workspace key={account.id} account={account} onLogout={() => setAccount(null)} /> : <Auth onLogin={setAccount} />;
}

function Workspace({ account, onLogout }: { account: Account; onLogout: () => void }) {
  const [spaces, setSpaces] = useState<Space[]>([]), [selected, setSelected] = useState(''), [spacesReady, setSpacesReady] = useState(false);
  const [page, setPage] = useState<'timeline' | 'calendar' | 'activity'>('timeline');
  const [query, setQuery] = useState(''), [tab, setTab] = useState('all'), [priority, setPriority] = useState('all');
  const [sort, setSort] = useState('deadline'), [type, setType] = useState('all'), [mobileNav, setMobileNav] = useState(false);
  const [spaceDialog, setSpaceDialog] = useState<'create' | 'join' | null>(null), [teamOpen, setTeamOpen] = useState(false), [helpOpen, setHelpOpen] = useState(false);
  const [editor, setEditor] = useState<Item | 'new' | null>(null), [error, setError] = useState('');
  const [toasts, setToasts] = useState<{ id: string; message: string }[]>([]);
  const toastTimers = useRef<ReturnType<typeof setTimeout>[]>([]);
  const notify = useCallback((message: string) => {
    const id = crypto.randomUUID(); setToasts(old => [...old.slice(-2), { id, message }]);
    toastTimers.current.push(setTimeout(() => setToasts(old => old.filter(t => t.id !== id)), 5500));
  }, []);
  useEffect(() => () => toastTimers.current.forEach(clearTimeout), []);
  const onActivity = useCallback((activity: Activity) => { if (activity.actor.id !== account.id) notify(activity.message); }, [account.id, notify]);
  const timeline = useTimeline(selected, onActivity);
  const writer = useWriter(account.id);

  useEffect(() => {
    let active = true;
    async function loadSpaces() {
      try {
        const result = await api<Space[]>('/spaces'); if (!active) return;
        setSpaces(result); setSpacesReady(true);
        setSelected(current => {
          if (result.some(s => s.id === current)) return current;
          let remembered = ''; try { remembered = localStorage.getItem(`timeline-space-${account.id}`) || ''; } catch { /* optional */ }
          return result.find(s => s.id === remembered)?.id || result[0]?.id || '';
        });
      } catch (error) { if (active) { setError(messageOf(error)); setSpacesReady(true); } }
    }
    loadSpaces(); const interval = setInterval(loadSpaces, 10000);
    return () => { active = false; clearInterval(interval); };
  }, [account.id]);
  useEffect(() => {
    setEditor(null); setTeamOpen(false); setQuery(''); setTab('all'); setPriority('all');
    if (selected) try { localStorage.setItem(`timeline-space-${account.id}`, selected); } catch { /* optional */ }
  }, [selected, account.id]);
  useEffect(() => {
    const keydown = (event: KeyboardEvent) => { if ((event.ctrlKey || event.metaKey) && event.key === 'k') { event.preventDefault(); document.getElementById('timeline-search')?.focus(); } };
    window.addEventListener('keydown', keydown); return () => window.removeEventListener('keydown', keydown);
  }, []);
  useEffect(() => {
    if (!mobileNav) return;
    document.querySelector<HTMLButtonElement>('.sidebar nav button')?.focus();
    const close = (event: KeyboardEvent) => { if (event.key === 'Escape') setMobileNav(false); };
    window.addEventListener('keydown', close);
    return () => { window.removeEventListener('keydown', close); document.querySelector<HTMLButtonElement>('.mobile-menu')?.focus(); };
  }, [mobileNav]);
  // Tick date-dependent views even if the team is quiet.
  const [, tick] = useState(0);
  useEffect(() => { const timer = setInterval(() => tick(n => n + 1), 60000); return () => clearInterval(timer); }, []);
  const space = timeline.space || spaces.find(s => s.id === selected);
  const live = timeline.connection === 'live';
  const canEdit = live && (space?.role === 'owner' || space?.role === 'editor');
  const canWrite = canEdit && !writer.busy && !writer.pending;
  const items = timeline.items;
  const open = items.filter(i => i.status === 'open'), done = items.filter(i => i.status === 'done'), late = items.filter(overdue);
  const percentage = items.length ? Math.round(done.length / items.length * 100) : 0;
  const filtered = items.filter(item => {
    if (tab === 'open' && item.status !== 'open') return false;
    if (tab === 'done' && item.status !== 'done') return false;
    if (tab === 'overdue' && !overdue(item)) return false;
    return (priority === 'all' || item.priority === priority) && (type === 'all' || item.type === type) && `${item.title} ${item.description}`.toLowerCase().includes(query.toLowerCase());
  }).sort((a, b) => {
    const rank = { high: 0, medium: 1, low: 2 };
    if (sort === 'priority') return rank[a.priority] - rank[b.priority] || itemDate(a).localeCompare(itemDate(b));
    return itemDate(a).localeCompare(itemDate(b)) || (a.deadline.at || '').localeCompare(b.deadline.at || '') || rank[a.priority] - rank[b.priority];
  });
  function choosePage(next: typeof page) { setPage(next); setMobileNav(false); }
  function addedSpace(next: Space) { setSpaces(old => [...old.filter(s => s.id !== next.id), next]); setSelected(next.id); setPage('timeline'); notify(`You’re in ${next.name}. Make yourself at home.`); }
  async function toggle(item: Item) {
    if (!canWrite) return;
    try {
      await writer.write({ path: `/spaces/${selected}/items/${item.id}`, method: 'PUT', body: { operation_id: crypto.randomUUID(), base_version: item.version, type: item.type, title: item.title, description: item.description, deadline: item.deadline, priority: item.priority, status: item.status === 'done' ? 'open' : 'done' } });
      notify(item.status === 'done' ? 'Back on your list.' : 'One more thing done. Nicely done.');
    } catch (error) { setError(messageOf(error)); }
  }
  async function retryWrite() {
    if (!writer.pending) return;
    try { await writer.write(writer.pending); setError(''); setEditor(null); notify('Your change is confirmed.'); timeline.reconnect(); }
    catch (error) { setError(messageOf(error)); }
  }
  async function logout() {
    try { await api('/auth/logout', 'POST', {}); onLogout(); } catch (error) { setError(messageOf(error)); }
  }
  return <div className="app-shell">
    {mobileNav && <button className="nav-scrim" aria-label="Close navigation" onClick={() => setMobileNav(false)} />}
    <aside className={`sidebar ${mobileNav ? 'mobile-open' : ''}`}>
      <a href="/" className="brand"><span className="brand-icon"><Layers3 size={22} /></span>sequence<span className="brand-dot">.</span></a>
      <div className="space-select-wrap"><span className="space-symbol">{space?.name.slice(0, 1).toUpperCase() || 'Y'}</span><div><span className="eyebrow">YOUR SPACE</span><select aria-label="Current space" value={selected} onChange={e => { setSelected(e.target.value); setMobileNav(false); }}><option value="" disabled>{spacesReady ? 'Choose a space' : 'Loading…'}</option>{spaces.map(s => <option key={s.id} value={s.id}>{s.name}</option>)}</select></div><ChevronDown size={14} /></div>
      <span className="nav-label">WORKSPACE</span>
      <nav aria-label="Main navigation"><button className={page === 'timeline' ? 'active' : ''} onClick={() => choosePage('timeline')}><LayoutList size={18} />Timeline<span className="nav-count">{open.length}</span></button><button className={page === 'calendar' ? 'active' : ''} onClick={() => choosePage('calendar')}><CalendarDays size={18} />Calendar</button><button className={page === 'activity' ? 'active' : ''} onClick={() => choosePage('activity')}><ActivityIcon size={18} />Activity{timeline.activities.length > 0 && <span className="nav-dot" />}</button></nav>
      <div className="sidebar-divider" /><span className="nav-label">MAKE IT YOURS</span><nav aria-label="Space actions"><button onClick={() => { setSpaceDialog('create'); setMobileNav(false); }}><Plus size={18} />Create a space</button><button onClick={() => { setSpaceDialog('join'); setMobileNav(false); }}><Users size={18} />Join a space</button>{space?.role === 'owner' && <button onClick={() => { setTeamOpen(true); setMobileNav(false); }}><Settings2 size={18} />Manage access</button>}</nav>
      <div className="sidebar-bottom"><div className="sidebar-note"><span className="sun-drawing" aria-hidden="true">✳</span><p>A little planning.<br /><strong>A clearer tomorrow.</strong></p></div><button className="help-button" onClick={() => setHelpOpen(true)}><CircleHelp size={17} /> A quick guide <ArrowRight size={14} /></button><div className="profile"><span className="avatar account-avatar">{account.display_name.slice(0, 1).toUpperCase()}</span><div><strong>{account.display_name}</strong><span>@{account.username}</span></div><button className="icon-button" title="Sign out" aria-label="Sign out" onClick={logout}><LogOut size={17} /></button></div></div>
    </aside>
    <div className="main-shell"><header className="topbar"><div className="breadcrumb"><button className="icon-button mobile-menu" aria-label="Open navigation" onClick={() => setMobileNav(true)}><Menu size={21} /></button><span>{space?.name || 'Your workspace'}</span><span className="breadcrumb-slash">/</span><strong>{page === 'calendar' ? 'Calendar' : page === 'activity' ? 'Activity' : 'Timeline'}</strong></div><div className="topbar-actions"><label className="search-box"><Search size={16} /><input id="timeline-search" aria-label="Search items" placeholder="Find something…" value={query} onChange={e => { setQuery(e.target.value); if (page === 'activity') setPage('timeline'); }} /><kbd>⌘ K</kbd></label><span className="topbar-avatar">{account.display_name.slice(0, 1).toUpperCase()}</span></div></header>
      <main className="workspace-main">
        <div className="page-intro"><div><span className="eyebrow">{new Date().toLocaleDateString(undefined, { weekday: 'long', month: 'long', day: 'numeric' }).toUpperCase()}</span><h1>{page === 'activity' ? 'Every step, together.' : page === 'calendar' ? 'See the bigger picture.' : 'Make room for what matters.'}</h1><p>{page === 'activity' ? 'A shared story of progress, one update at a time.' : 'Your plans in one place. A little more clarity for the days ahead.'}</p></div>{space && <div className="intro-actions">{space.role === 'owner' && <button className="button secondary" onClick={() => setTeamOpen(true)}><Users size={16} />Invite people</button>}{space.role !== 'viewer' && <button className="button primary" disabled={!canWrite} onClick={() => setEditor('new')}><Plus size={18} />New item</button>}</div>}</div>
        {error && <div className="error-note page-error" role="alert">{error}<button className="icon-button" onClick={() => setError('')} aria-label="Dismiss error"><X size={16} /></button></div>}
        {writer.pending && !writer.busy && <div className="connection-banner"><WifiOff size={18} /><div><strong>A save is waiting for confirmation.</strong><p>Retry the original change safely before making another edit.</p></div><button className="button secondary" onClick={retryWrite}>Confirm change</button></div>}
        {selected && !live && <div className="connection-banner"><WifiOff size={18} /><div><strong>{timeline.connection === 'denied' ? 'This space is no longer available to you.' : timeline.connection === 'connecting' ? 'Connecting to your timeline…' : 'Reconnecting. Your saved plans are safe.'}</strong><p>{timeline.connection === 'denied' ? 'Choose another space or request access again.' : 'Editing will be available when the live connection returns.'}</p></div>{timeline.connection === 'denied' && <button className="button secondary" onClick={timeline.reconnect}>Check access</button>}</div>}
        {!space ? <section className="onboarding"><div className="onboarding-art" aria-hidden="true"><span className="orbit orbit-one" /><span className="orbit orbit-two" /><span className="onboarding-mark"><Layers3 size={38} strokeWidth={1.4} /></span><i className="orbit-dot one" /><i className="orbit-dot two" /></div><span className="eyebrow">A FRESH START</span><h2>{spacesReady ? 'Big plans start with a little space.' : 'Gathering your spaces…'}</h2><p>Create a timeline for your team, your next project, or simply yourself. Everything you need to move forward, together.</p><div className="onboarding-actions"><button className="button primary" onClick={() => setSpaceDialog('create')}><Plus size={17} />Create your first space</button><button className="button secondary" onClick={() => setSpaceDialog('join')}><Users size={17} />Join a space</button></div><p className="small muted">Waiting for approval? Your space will appear here automatically.</p></section> : <>
          <div className="stats-grid"><button className={`stat-card ${tab === 'open' ? 'stat-selected' : ''}`} onClick={() => { setTab(tab === 'open' ? 'all' : 'open'); setPage('timeline'); }}><span className="stat-icon green"><ListIcon /></span><div><span className="stat-label">On your horizon</span><strong>{open.length}<small>open {open.length === 1 ? 'item' : 'items'}</small></strong></div><ArrowRight size={17} /></button><button className={`stat-card ${tab === 'overdue' ? 'stat-selected' : ''}`} onClick={() => { setTab(tab === 'overdue' ? 'all' : 'overdue'); setPage('timeline'); }}><span className="stat-icon amber"><Clock3 size={20} /></span><div><span className="stat-label">Needs a little attention</span><strong>{late.length}<small>overdue</small></strong></div><ArrowRight size={17} /></button><button className={`stat-card ${tab === 'done' ? 'stat-selected' : ''}`} onClick={() => { setTab(tab === 'done' ? 'all' : 'done'); setPage('timeline'); }}><span className="stat-icon purple"><CheckCheck size={20} /></span><div><span className="stat-label">Look how far you’ve come</span><strong>{done.length}<small>completed</small></strong></div><ArrowRight size={17} /></button></div>
          <div className={`content-grid ${page === 'activity' ? 'activity-layout' : ''}`}><section className="timeline-panel"><div className="panel-heading"><div><h2>{page === 'activity' ? 'The latest happenings' : page === 'calendar' ? 'Your calendar' : 'Your timeline'}<span className="count">{page === 'activity' ? timeline.activities.length : items.length}</span></h2><p>{page === 'activity' ? 'Changes made by you and your people.' : space.role === 'viewer' ? 'You have viewing access to this shared space.' : 'Small steps. Shared progress.'}</p></div><span className={`live-label ${!live ? 'offline' : ''}`}><i />{live ? 'Live' : 'Connecting'}</span></div>
          {page === 'activity' ? <ActivityFeed key={selected} full spaceID={selected} activities={timeline.activities} /> : <><div className="timeline-toolbar"><div className="tabs" aria-label="Item status">{[['all', 'All items'], ['open', 'Upcoming'], ['done', 'Completed']].map(([value, label]) => <button key={value} className={tab === value ? 'active' : ''} onClick={() => setTab(value)}>{label}</button>)}</div><div className="view-switch"><button className={page === 'timeline' ? 'selected' : ''} aria-label="List view" onClick={() => setPage('timeline')}><LayoutList size={17} /></button><button className={page === 'calendar' ? 'selected' : ''} aria-label="Calendar view" onClick={() => setPage('calendar')}><CalendarDays size={17} /></button></div></div><div className="filter-row"><label><Flag size={13} /><select aria-label="Filter priority" value={priority} onChange={e => setPriority(e.target.value)}><option value="all">All priorities</option><option value="high">High priority</option><option value="medium">Medium priority</option><option value="low">Low priority</option></select></label><label><Layers3 size={13} /><select aria-label="Filter item type" value={type} onChange={e => setType(e.target.value)}><option value="all">Tasks & events</option><option value="task">Tasks</option><option value="event">Events</option></select></label><label className="sort-select"><ArrowDownWideNarrow size={14} /><select aria-label="Sort items" value={sort} onChange={e => setSort(e.target.value)}><option value="deadline">By deadline</option><option value="priority">By priority</option></select></label>{(query || tab === 'overdue') && <button className="text-button" onClick={() => { setQuery(''); setTab('all'); }}>Clear {query ? 'search' : 'overdue filter'}<X size={12} /></button>}</div><TimelineView items={filtered} canEdit={canWrite} onEdit={setEditor} onToggle={toggle} onCreate={() => setEditor('new')} calendar={page === 'calendar'} /></>}
          </section><aside className="right-rail"><section className="progress-card"><div><span className="eyebrow">LITTLE WINS ADD UP</span><Sparkles size={17} /></div><div className="progress-main"><div className="progress-ring"><svg viewBox="0 0 80 80" aria-hidden="true"><circle cx="40" cy="40" r="33" /><circle cx="40" cy="40" r="33" strokeDasharray={`${percentage * 2.074} 207.4`} /></svg><span>{percentage}<small>%</small></span></div><div><h3>{percentage === 100 && items.length ? 'All caught up.' : 'Moving forward.'}</h3><p>{done.length} of {items.length} items done.<br />{items.length ? 'Every step counts.' : 'Your first step is waiting.'}</p></div></div><div className="progress-bottom"><span className="tiny-dot" /> A little better, together.</div></section>{page !== 'activity' && <section className="activity-panel"><div className="rail-heading"><h2>Recent activity</h2><span className="activity-live-dot" /></div><ActivityFeed key={selected} spaceID={selected} activities={timeline.activities} /><button className="activity-all" onClick={() => setPage('activity')}>See all activity<ArrowRight size={15} /></button></section>}<div className="shared-note"><Users size={18} /><p>A shared timeline.<br /><strong>Everyone on the same page.</strong></p></div></aside></div>
        </>}
        <footer className="workspace-footer"><span>Made for a little more clarity.</span><span><span className="tiny-dot" /> One plan. In sync.</span></footer>
      </main>
    </div>
    {spaceDialog && <SpaceDialog mode={spaceDialog} onClose={() => setSpaceDialog(null)} onSpace={addedSpace} />}
    {teamOpen && space?.role === 'owner' && <TeamDialog key={selected} space={space} onClose={() => setTeamOpen(false)} />}
    {editor && space && <ItemEditor key={editor === 'new' ? 'new' : editor.id} spaceID={selected} item={editor === 'new' ? undefined : editor} canEdit={canEdit} blocked={!!writer.pending && !writer.busy} busy={writer.busy} write={writer.write} onClose={() => setEditor(null)} />}
    {helpOpen && <Modal title="A little guide to Timeline." subtitle="Everything you need to find your rhythm." onClose={() => setHelpOpen(false)}><div className="guide-list"><p><strong>1. Make a space.</strong> Keep a project or part of life together. Invite people using its key.</p><p><strong>2. Give your plans a place.</strong> Add a task or event, a deadline, and a priority. Check it off when it’s done.</p><p><strong>3. Move forward together.</strong> Admins choose who can edit. Updates arrive live, and activity keeps everyone in the loop.</p><p className="muted">Timed deadlines use your device’s local time. Date-only deadlines stay on the date you choose.</p></div><button className="button primary button-full" onClick={() => setHelpOpen(false)}>Let’s make a plan<ArrowRight size={16} /></button></Modal>}
    <div className="toast-stack" aria-live="polite">{toasts.map(toast => <div className="toast" key={toast.id}><span><Check size={15} /></span><p>{toast.message}</p><button className="icon-button" aria-label="Dismiss notification" onClick={() => setToasts(old => old.filter(t => t.id !== toast.id))}><X size={14} /></button></div>)}</div>
  </div>;
}
function ListIcon() { return <LayoutList size={20} />; }
