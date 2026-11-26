import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, LogOut, ScanLine, ShieldCheck, Undo2 } from 'lucide-react';
import type { ModuleContext } from '../../platform/microfrontends/contracts';
import { useI18n } from '../../core/i18n';
import { Alert, Badge, Button, Card, Input } from '../../design-system/atoms';
import { EmptyState, PageHeading, SelectMenu, StatusLine } from '../../design-system/molecules';
import { CameraQrScanner } from './CameraQrScanner';
import { readSession } from '../../core/session';
import { CertificatesPanel, DevicesPanel, ReasonModal, ReportsPanel, SessionsPanel, type OnsiteApi } from './OnsitePanels';
import { onsiteReason, trOnsite as tr } from '../../core/onsite-i18n';

type Tab = 'checkin' | 'attendance' | 'sessions' | 'devices' | 'certificates' | 'reports';
type Ticket = { ticket_id: string; masked_holder: string; status: string; provisional?: boolean; verified?: boolean };
type Dashboard = { registered: number; confirmed: number; present: number; attendance_rate: number };
type AttendanceFact = { id: string; ticket_id: string; type: string; occurred_at: string; effective_status: string; reason?: string; compensates_fact_id?: string };
type OperableEvent = { id: string; name: string; status: string; starts_at: string; ends_at: string };
type RosterEntry = { registration_id: string; ticket_id: string; display_name: string; state: string; checked_in_at?: string | null; checked_out_at?: string | null };
const tabByRoute: Record<string, Tab> = { '/onsite/attendance': 'attendance', '/onsite/sessions': 'sessions', '/onsite/devices': 'devices', '/onsite/certificates': 'certificates', '/onsite/reports': 'reports' };

export function OnsiteModule({ context }: { context: ModuleContext }) {
  const { t } = useI18n();
  const navigate = (path: string) => { window.history.pushState({}, '', path); window.dispatchEvent(new PopStateEvent('popstate')); };
  const [message, setMessage] = useState('');
  const [loading, setLoading] = useState(false);
  const [searching, setSearching] = useState(false);
  const [cameraOpen, setCameraOpen] = useState(false);
  const [event, setEvent] = useState('');
  const [events, setEvents] = useState<OperableEvent[]>([]);
  const [code, setCode] = useState('');
  const [ticket, setTicket] = useState<Ticket | null>(null);
  const [dashboard, setDashboard] = useState<Dashboard>({ registered: 0, confirmed: 0, present: 0, attendance_rate: 0 });
  const [facts, setFacts] = useState<AttendanceFact[]>([]);
  const [roster, setRoster] = useState<RosterEntry[]>([]);
  const [pendingToken, setPendingToken] = useState('');
  const [voiding, setVoiding] = useState<AttendanceFact | null>(null);
  const tab: Tab = tabByRoute[context.route ?? ''] || 'checkin';
  const eventId = event;
  const operationsApiUrl = import.meta.env.VITE_OPERATIONS_API_URL || 'http://localhost:8083';
  // role sólo cambia los claims simulados del modo demo; con token real decide el backend.
  const authHeaders = (extra: Record<string, string> = {}, role: 'staff' | 'supervisor' | 'manager' = 'staff'): Record<string, string> => {
    const session = readSession();
    if (import.meta.env.DEV && session?.access_token === 'test') return { 'X-Mock-Role': role, 'X-Mock-Tenant': context.tenantId, ...extra };
    return session?.access_token ? { Authorization: `Bearer ${session.access_token}`, ...extra } : { 'X-Mock-Role': role, 'X-Mock-Tenant': context.tenantId, ...extra };
  };
  const api: OnsiteApi = { onsite: context.apiBaseUrl, operations: operationsApiUrl, eventId, headers: authHeaders };

  const loadDashboard = async () => {
    if (!eventId) return;
    try {
      const response = await fetch(`${context.apiBaseUrl}/onsite/v1/dashboard?event_id=${encodeURIComponent(eventId)}`, { headers: authHeaders() });
      if (!response.ok) throw new Error();
      setDashboard(await response.json());
    } catch { setMessage(t('onsite.dashboardError')); }
  };
  const loadAttendance = async () => {
    if (!eventId) return;
    try {
      const response = await fetch(`${context.apiBaseUrl}/onsite/v1/attendance?event_id=${encodeURIComponent(eventId)}&scope=main`, { headers: authHeaders() });
      if (!response.ok) throw new Error();
      const data = await response.json();
      setFacts(data.items || []);
      const rosterResponse = await fetch(`${context.apiBaseUrl}/onsite/v1/events/${encodeURIComponent(eventId)}/roster?scope=main`, { headers: authHeaders() });
      if (rosterResponse.ok) setRoster((await rosterResponse.json()).items || []);
    } catch { setMessage(t('onsite.apiError')); }
  };
  const loadEvents = async () => {
    try {
      const response = await fetch(`${context.apiBaseUrl}/onsite/v1/events`, { headers: authHeaders() });
      if (!response.ok) throw new Error();
      const items: OperableEvent[] = (await response.json()).items || [];
      setEvents(items);
      setEvent((current) => current && items.some((item) => item.id === current) ? current : (items[0]?.id || ''));
    } catch { setMessage(t('onsite.apiError')); }
  };
  useEffect(() => { void loadEvents(); }, []);
  useEffect(() => { if (tab === 'checkin' && eventId) void loadDashboard(); }, [tab, eventId]);
  useEffect(() => { if (tab === 'attendance') void loadAttendance(); }, [tab, eventId]);

  const search = async (value = code) => {
    const query = value.trim();
    if (query.length < 4) { setMessage(t('onsite.searchEmpty')); setTicket(null); return; }
    setSearching(true); setMessage(''); setTicket(null);
    try {
      const response = await fetch(`${context.apiBaseUrl}/onsite/v1/tickets/search?q=${encodeURIComponent(query)}&event_id=${encodeURIComponent(eventId)}&scope=main`, { headers: authHeaders() });
      if (!response.ok) throw new Error();
      const items: Ticket[] = await response.json();
      if (!items.length) setMessage(t('onsite.searchEmpty')); else setTicket(items[0]);
    } catch { setMessage(t('onsite.apiError')); } finally { setSearching(false); }
  };
  const checkin = async (ticketId: string) => {
    setLoading(true); setMessage('');
    try {
      const response = await fetch(`${context.apiBaseUrl}/onsite/v1/attendance/check-ins`, { method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() }), body: JSON.stringify({ ticket_id: ticketId, event_id: eventId, scope: 'main' }) });
      const data = await response.json();
      if (!response.ok) { setMessage(t('onsite.checkinError', { error: onsiteReason(data.code) })); return; }
      setMessage(t('onsite.checkinSuccess')); setTicket(null); setCode(''); await loadDashboard();
    } catch { setMessage(t('onsite.apiError')); } finally { setLoading(false); }
  };
  // Un QR firmado se verifica en el backend (firma Ed25519, evento, zona, vigencia y revocación).
  const verifyQr = async (token: string) => {
    setSearching(true); setMessage(''); setTicket(null); setPendingToken('');
    try {
      const response = await fetch(`${context.apiBaseUrl}/onsite/v1/scans`, { method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json' }), body: JSON.stringify({ token, event_id: eventId, scope: 'main' }) });
      const data = await response.json();
      if (!response.ok) { setMessage(onsiteReason(data.code)); return; }
      if (data.accepted) { setTicket({ ticket_id: data.ticket.ticket_id, masked_holder: data.ticket.masked_holder, status: 'active', provisional: data.provisional, verified: true }); setCode(data.ticket.ticket_id); return; }
      setMessage(`No apto: ${onsiteReason(data.reason)}`);
      // Un ticket recién emitido puede aceptarse como provisional si supervisión lo autoriza.
      if (data.reason === 'ticket_not_materialized') {
        const asSupervisor = await fetch(`${context.apiBaseUrl}/onsite/v1/scans`, { method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json' }, 'supervisor'), body: JSON.stringify({ token, event_id: eventId, scope: 'main' }) }).then((r) => r.json()).catch(() => ({}));
        if (asSupervisor.can_resolve) setPendingToken(token);
      }
    } catch { setMessage(t('onsite.apiError')); } finally { setSearching(false); }
  };
  const resolveProvisional = async () => {
    setLoading(true);
    try {
      const response = await fetch(`${context.apiBaseUrl}/onsite/v1/scans/provisional`, { method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() }, 'supervisor'), body: JSON.stringify({ token: pendingToken, event_id: eventId, scope: 'main' }) });
      const data = await response.json();
      if (!response.ok) { setMessage(onsiteReason(data.code)); return; }
      setPendingToken(''); setMessage(tr('Ticket provisional aceptado. Ya puedes registrar su entrada.'));
      setTicket({ ticket_id: data.ticket_id, masked_holder: data.masked_holder, status: 'active', provisional: true, verified: true });
    } catch { setMessage(t('onsite.apiError')); } finally { setLoading(false); }
  };
  const consumeQr = (rawValue: string) => {
    const compact = rawValue.trim();
    const match = compact.match(/^nexo:ticket:(.+)$/i);
    setCameraOpen(false);
    if (match) { setCode(match[1]); void search(match[1]); return; }
    if (compact.split('.').length === 3) { void verifyQr(compact); return; }
    setMessage(t('onsite.qrInvalid'));
  };
  const attendanceAction = async (url: string, body: unknown, role: 'staff' | 'supervisor', ok: string) => {
    try {
      const response = await fetch(url, { method: 'POST', headers: authHeaders({ 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() }, role), body: JSON.stringify(body) });
      const data = await response.json().catch(() => ({}));
      if (!response.ok) { setMessage(onsiteReason(data.code)); return false; }
      setMessage(ok); await loadAttendance(); return true;
    } catch { setMessage(t('onsite.apiError')); return false; }
  };
  const voided = new Set(facts.filter((fact) => fact.type === 'void').map((fact) => fact.compensates_fact_id));
  const error = message.startsWith('No ') || message.startsWith('No se') || message.startsWith('API') || message.startsWith('Could not') || message.startsWith('La ') || message.startsWith('El ') || message.startsWith('Ya ') || message.startsWith('Ese ') || message.startsWith('Esta ') || message.startsWith('The ') || message.startsWith('This ');

  return <div className="nx-module-page">
    <PageHeading title={tab === 'checkin' ? t('onsite.checkinTab') : tab === 'attendance' ? t('onsite.attendanceTab') : tab === 'sessions' ? tr('Sesiones') : tab === 'devices' ? tr('Dispositivos y turnos') : tab === 'certificates' ? tr('Certificados') : tr('Reportes')} description={tab === 'checkin' ? t('onsite.registerDescription') : tab === 'attendance' ? t('onsite.attendanceDescription') : tab === 'reports' || tab === 'certificates' ? t('onsite.reportsDescription') : undefined} action={<SelectMenu label={t('onsite.event')} value={event} onChange={setEvent} options={events.map((item) => ({ value: item.id, label: item.name }))} />} />
    {tab === 'checkin' && <div className="nx-content-grid">
      <Card className="nx-scan-panel">
        <div className="nx-panel-heading"><div><p className="nx-eyebrow">{t('onsite.station')}</p><h2>{t('onsite.register')}</h2><p className="nx-muted">{t('onsite.registerDescription')}</p></div><Badge tone="success"><CheckCircle2 size={14} /> {t('onsite.ready')}</Badge></div>
        <div className="nx-scanner"><ScanLine size={42} /><strong>{t('onsite.scanner')}</strong><span>{t('onsite.scannerDescription')}</span></div>
        <div className="nx-ticket-entry"><Input label={t('onsite.ticketCode')} help={t('onsite.ticketCodeHelp')} value={code} onChange={(event) => setCode(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void search(); }} placeholder="DEMO-2026-001" /><div className="nx-actions"><Button onClick={() => setCameraOpen(true)}><ScanLine size={16} />{t('onsite.openCamera')}</Button><Button variant="secondary" onClick={() => consumeQr(code)} loading={searching}>{t('onsite.scan')}</Button><Button variant="secondary" onClick={() => void search()} loading={searching}>{t('onsite.search')}</Button></div></div>
        {pendingToken && <Alert tone="warning"><ShieldCheck size={16} /> {tr('Acción de supervisión')} <Button variant="secondary" onClick={() => void resolveProvisional()} loading={loading}>{tr('Aceptar como provisional')}</Button></Alert>}
        {ticket && <div className="nx-ticket-result"><div><p className="nx-eyebrow">{t('onsite.searchSelect')}{ticket.verified ? ` · ${tr('QR verificado')}` : ''}</p><strong>{ticket.masked_holder} {ticket.provisional && <Badge tone="warning">{tr('Provisional')}</Badge>}</strong><span>{ticket.ticket_id} · {ticket.status === 'active' ? t('onsite.ticketReady') : t('onsite.ticketRejected')}</span></div><Button onClick={() => void checkin(ticket.ticket_id)} loading={loading}>{t('onsite.registerSelected')}</Button></div>}
        {message && <Alert tone={error ? 'danger' : 'success'}>{error ? <AlertTriangle size={16} /> : <CheckCircle2 size={16} />} {message}</Alert>}
      </Card>
      <Card className="nx-readiness"><div className="nx-panel-heading"><div><p className="nx-eyebrow">{t('onsite.attendanceTitle')}</p><h2>{t('onsite.attendanceSummary', { percentage: `${Math.round(dashboard.attendance_rate)}%` })}</h2></div></div><StatusLine label={t('onsite.registered')} value={String(dashboard.registered)} /><StatusLine label={t('onsite.confirmed')} value={String(dashboard.confirmed)} tone="success" /><StatusLine label={t('onsite.present')} value={String(dashboard.present)} tone="success" /></Card>
    </div>}
    {tab === 'attendance' && <Card>{roster.length === 0 ? <EmptyState title={t('onsite.attendanceTab')} description={t('onsite.history')} action={<Button variant="secondary" onClick={() => navigate('/onsite')}>{t('onsite.registerEntry')}</Button>} /> : <div className="nx-history-list"><div className="nx-panel-heading"><div><p className="nx-eyebrow">{t('onsite.attendanceTitle')}</p><h2>{roster.length} {t('onsite.entries')}</h2></div><Button variant="secondary" onClick={() => void loadAttendance()}>Actualizar</Button></div>{roster.map((participant) => <div className="nx-history-row" key={participant.registration_id}><div><strong>{participant.display_name}</strong><span>{participant.ticket_id} · {participant.state}{participant.checked_in_at ? ` · ${new Date(participant.checked_in_at).toLocaleString()}` : ''}</span></div><div className="nx-actions"><Badge tone={participant.state === 'present' ? 'success' : 'warning'}>{participant.state}</Badge></div></div>)}<div className="nx-panel-heading"><div><p className="nx-eyebrow">{t('onsite.history')}</p><h2>{facts.length} {t('onsite.entries')}</h2></div></div>{facts.map((fact) => <div className="nx-history-row" key={fact.id}><div><strong>{fact.ticket_id}</strong><span>{fact.type} · {new Date(fact.occurred_at).toLocaleString()}{fact.reason ? ` · ${fact.reason}` : ''}</span></div><div className="nx-actions">{voided.has(fact.id) ? <Badge tone="danger">{tr('Anulado')}</Badge> : <Badge tone={fact.type === 'void' ? 'warning' : 'success'}>{fact.effective_status}</Badge>}
          {fact.type === 'check_in' && fact.effective_status === 'present' && !voided.has(fact.id) && <Button variant="ghost" onClick={() => void attendanceAction(`${context.apiBaseUrl}/onsite/v1/attendance/check-outs`, { ticket_id: fact.ticket_id, event_id: eventId, scope: 'main' }, 'staff', tr('Salida registrada.'))}><LogOut size={16} />{tr('Registrar salida')}</Button>}
          {fact.type !== 'void' && !voided.has(fact.id) && <Button variant="ghost" onClick={() => setVoiding(fact)}><Undo2 size={16} />{tr('Anular')}</Button>}
        </div></div>)}</div>}</Card>}
    {tab === 'attendance' && message && <Alert tone={error ? 'danger' : 'success'}>{error ? <AlertTriangle size={16} /> : <CheckCircle2 size={16} />} {message}</Alert>}
    {tab === 'sessions' && <SessionsPanel api={api} />}
    {tab === 'devices' && <DevicesPanel api={api} />}
    {tab === 'certificates' && <CertificatesPanel api={api} />}
    {tab === 'reports' && <ReportsPanel api={api} />}
    <ReasonModal open={!!voiding} title={`${tr('Anular registro')} · ${voiding?.ticket_id || ''}`} description={tr('La anulación queda auditada. Si anulas una salida, la persona vuelve a figurar presente.')} onCancel={() => setVoiding(null)} onConfirm={async (reason) => { if (voiding && await attendanceAction(`${context.apiBaseUrl}/onsite/v1/attendance/facts/${voiding.id}/voids`, { reason }, 'supervisor', tr('Registro anulado.'))) setVoiding(null); }} />
    <CameraQrScanner open={cameraOpen} onClose={() => setCameraOpen(false)} onDetected={consumeQr} copy={{ title: t('onsite.cameraTitle'), description: t('onsite.cameraDescription'), starting: t('onsite.cameraStarting'), ready: t('onsite.cameraReady'), error: t('onsite.cameraError'), stop: t('onsite.stopCamera') }} />
  </div>;
}
