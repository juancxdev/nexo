import { useEffect, useState } from 'react';
import { AlertTriangle, CheckCircle2, Download, FileText, RefreshCw, ShieldCheck, Trash2, Users } from 'lucide-react';
import { Alert, Badge, Button, Card, Input, Modal } from '../../design-system/atoms';
import { EmptyState, SelectMenu, StatusLine } from '../../design-system/molecules';
import { onsiteReason, trOnsite as tr } from '../../core/onsite-i18n';

/** Acceso a las APIs que comparten los paneles. role sólo aplica en modo demo (claims simulados). */
export type OnsiteApi = {
  onsite: string;
  operations: string;
  eventId: string;
  headers: (extra?: Record<string, string>, role?: 'staff' | 'supervisor' | 'manager') => Record<string, string>;
};

type Notice = { tone: 'success' | 'danger'; text: string } | null;

async function send(url: string, init: RequestInit): Promise<{ ok: boolean; status: number; data: any }> {
  const response = await fetch(url, init);
  const text = await response.text();
  let data: any = {};
  try { data = text ? JSON.parse(text) : {}; } catch { data = {}; }
  return { ok: response.ok, status: response.status, data };
}

function NoticeLine({ notice }: { notice: Notice }) {
  if (!notice) return null;
  return <Alert tone={notice.tone}>{notice.tone === 'danger' ? <AlertTriangle size={16} /> : <CheckCircle2 size={16} />} {notice.text}</Alert>;
}

/** Pide un motivo de al menos 5 caracteres antes de una acción auditada. */
export function ReasonModal({ open, title, description, onCancel, onConfirm }: { open: boolean; title: string; description: string; onCancel: () => void; onConfirm: (reason: string) => Promise<void> }) {
  const [reason, setReason] = useState('');
  const [busy, setBusy] = useState(false);
  useEffect(() => { if (open) setReason(''); }, [open]);
  const valid = reason.trim().length >= 5;
  return <Modal open={open} title={title} description={description} onClose={onCancel}>
    <Input label={tr('Motivo')} help={tr('Escribe el motivo (mínimo 5 caracteres).')} value={reason} onChange={(event) => setReason(event.target.value)} autoFocus />
    <div className="nx-actions">
      <Button variant="secondary" onClick={onCancel}>{tr('Cancelar')}</Button>
      <Button variant="danger" disabled={!valid} loading={busy} onClick={async () => { setBusy(true); try { await onConfirm(reason.trim()); } finally { setBusy(false); } }}>{tr('Confirmar')}</Button>
    </div>
  </Modal>;
}

// --- Sesiones (OO-015) ---------------------------------------------------------

type Session = { code: string; name: string; access: string; capacity: number; entries: number };

export function SessionsPanel({ api }: { api: OnsiteApi }) {
  const [sessions, setSessions] = useState<Session[]>([]);
  const [selected, setSelected] = useState('');
  const [code, setCode] = useState('');
  const [busy, setBusy] = useState(false);
  const [notice, setNotice] = useState<Notice>(null);
  const load = async () => {
    const r = await send(`${api.onsite}/onsite/v1/sessions?event_id=${encodeURIComponent(api.eventId)}`, { headers: api.headers() }).catch(() => null);
    if (!r?.ok) { setNotice({ tone: 'danger', text: onsiteReason(r?.data?.code) }); return; }
    setSessions(r.data.items || []);
    if (!selected && r.data.items?.length) setSelected(r.data.items[0].code);
  };
  useEffect(() => { void load(); }, [api.eventId]);
  const enter = async () => {
    const value = code.trim();
    if (!value || !selected) return;
    setBusy(true); setNotice(null);
    const body = value.split('.').length === 3 ? { event_id: api.eventId, token: value } : { event_id: api.eventId, ticket_id: value.replace(/^nexo:ticket:/i, '') };
    const r = await send(`${api.onsite}/onsite/v1/sessions/${encodeURIComponent(selected)}/entries`, { method: 'POST', headers: api.headers({ 'Content-Type': 'application/json', 'Idempotency-Key': crypto.randomUUID() }), body: JSON.stringify(body) }).catch(() => null);
    setBusy(false);
    if (!r?.ok) { setNotice({ tone: 'danger', text: onsiteReason(r?.data?.code) }); return; }
    setNotice({ tone: 'success', text: `${tr('Ingreso a la sesión registrado.')} ${r.data.masked_holder || ''}` }); setCode(''); void load();
  };
  const current = sessions.find((s) => s.code === selected);
  return <div className="nx-content-grid">
    <Card>
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">OO-015</p><h2>{tr('Sesiones')}</h2><p className="nx-muted">{tr('Controla el ingreso a talleres y charlas. No cambia la asistencia principal.')}</p></div></div>
      {sessions.length === 0 ? <EmptyState title={tr('Sesiones')} description={tr('No hay sesiones configuradas para este evento.')} /> : <>
        <SelectMenu label={tr('Sesiones')} value={selected} onChange={setSelected} options={sessions.map((s) => ({ value: s.code, label: s.name }))} />
        <Input label={tr('Código de ticket o QR')} value={code} onChange={(event) => setCode(event.target.value)} onKeyDown={(event) => { if (event.key === 'Enter') void enter(); }} placeholder="DEMO-2026-001" />
        <div className="nx-actions"><Button onClick={() => void enter()} loading={busy}><Users size={16} />{tr('Registrar ingreso')}</Button></div>
      </>}
      <NoticeLine notice={notice} />
    </Card>
    <Card className="nx-readiness">
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">{tr('Aforo')}</p><h2>{current ? current.name : tr('Sesiones')}</h2></div><Button variant="secondary" onClick={() => void load()}><RefreshCw size={16} />{tr('Actualizar')}</Button></div>
      {sessions.map((s) => <StatusLine key={s.code} label={`${s.name} · ${s.access === 'restricted' ? tr('Restringida') : tr('Abierta')}`} value={s.capacity ? `${s.entries} / ${s.capacity}` : `${s.entries} · ${tr('Sin aforo')}`} tone={s.capacity && s.entries >= s.capacity ? 'warning' : 'success'} />)}
    </Card>
  </div>;
}

// --- Dispositivos y turnos (OO-011/012) -------------------------------------------

type Device = { ID: string; Label: string; Status: string; ReleaseVersion?: string; RevokedAt?: string | null };
type Shift = { id: string; scope: string; device_label: string; status: string; opened_at: string; closed_at?: string | null };

export function DevicesPanel({ api }: { api: OnsiteApi }) {
  const [devices, setDevices] = useState<Device[]>([]);
  const [shifts, setShifts] = useState<Shift[]>([]);
  const [label, setLabel] = useState('');
  const [revoking, setRevoking] = useState<Device | null>(null);
  const [notice, setNotice] = useState<Notice>(null);
  const manager = (extra: Record<string, string> = {}) => api.headers(extra, 'manager');
  const load = async () => {
    const [d, s] = await Promise.all([
      send(`${api.onsite}/onsite/v1/devices`, { headers: manager() }).catch(() => null),
      send(`${api.onsite}/onsite/v1/shifts?event_id=${encodeURIComponent(api.eventId)}`, { headers: manager() }).catch(() => null),
    ]);
    if (d?.ok) setDevices(d.data.items || []);
    if (s?.ok) setShifts(s.data.items || []);
    if (!d?.ok || !s?.ok) setNotice({ tone: 'danger', text: onsiteReason((d?.data || s?.data)?.code) });
  };
  useEffect(() => { void load(); }, [api.eventId]);
  const act = async (url: string, init: RequestInit, ok: string) => {
    setNotice(null);
    const r = await send(url, init).catch(() => null);
    if (!r?.ok) { setNotice({ tone: 'danger', text: onsiteReason(r?.data?.code) }); return false; }
    setNotice({ tone: 'success', text: ok }); void load(); return true;
  };
  const openShift = () => act(`${api.onsite}/onsite/v1/shifts`, { method: 'POST', headers: api.headers({ 'Content-Type': 'application/json' }), body: JSON.stringify({ event_id: api.eventId, scope: 'main' }) }, tr('Abrir turno'));
  return <div className="nx-content-grid">
    <Card>
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">OO-011</p><h2>{tr('Dispositivos y turnos')}</h2><p className="nx-muted">{tr('Revocar un dispositivo cierra sus turnos abiertos.')}</p></div><Button variant="secondary" onClick={() => void load()}><RefreshCw size={16} />{tr('Actualizar')}</Button></div>
      <div className="nx-ticket-entry"><Input label={tr('Nombre del dispositivo')} value={label} onChange={(event) => setLabel(event.target.value)} placeholder="Tablet puerta 2" /><div className="nx-actions"><Button disabled={!label.trim()} onClick={async () => { if (await act(`${api.onsite}/onsite/v1/devices`, { method: 'POST', headers: manager({ 'Content-Type': 'application/json' }), body: JSON.stringify({ label: label.trim() }) }, tr('Registrar dispositivo'))) setLabel(''); }}>{tr('Registrar dispositivo')}</Button></div></div>
      {devices.length === 0 ? <p className="nx-muted">{tr('Sin dispositivos registrados.')}</p> : <div className="nx-history-list">{devices.map((d) => <div className="nx-history-row" key={d.ID}><div><strong>{d.Label}</strong><span>{d.ID.slice(0, 8)}{d.ReleaseVersion ? ` · ${d.ReleaseVersion}` : ''}</span></div><div className="nx-actions"><Badge tone={d.Status === 'active' ? 'success' : 'danger'}>{d.Status === 'active' ? tr('Activo') : tr('Revocado')}</Badge>{d.Status === 'active' && <Button variant="ghost" onClick={() => setRevoking(d)}>{tr('Revocar')}</Button>}</div></div>)}</div>}
      <NoticeLine notice={notice} />
    </Card>
    <Card className="nx-readiness">
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">OO-012</p><h2>{tr('Turnos')}</h2></div><Button onClick={() => void openShift()}>{tr('Abrir turno')}</Button></div>
      {shifts.length === 0 ? <p className="nx-muted">{tr('Sin turnos en este evento.')}</p> : <div className="nx-history-list">{shifts.map((s) => <div className="nx-history-row" key={s.id}><div><strong>{s.device_label} · {s.scope}</strong><span>{new Date(s.opened_at).toLocaleString()}{s.closed_at ? ` → ${new Date(s.closed_at).toLocaleTimeString()}` : ''}</span></div><div className="nx-actions"><Badge tone={s.status === 'open' ? 'success' : 'neutral'}>{s.status === 'open' ? tr('Abierto') : tr('Cerrado')}</Badge>{s.status === 'open' && <Button variant="ghost" onClick={() => void act(`${api.onsite}/onsite/v1/shifts/${s.id}/closures`, { method: 'POST', headers: api.headers() }, tr('Cerrar turno'))}>{tr('Cerrar turno')}</Button>}</div></div>)}</div>}
    </Card>
    <ReasonModal open={!!revoking} title={`${tr('Revocar')} · ${revoking?.Label || ''}`} description={tr('Revocar un dispositivo cierra sus turnos abiertos.')} onCancel={() => setRevoking(null)} onConfirm={async (reason) => { if (revoking && await act(`${api.onsite}/onsite/v1/devices/${revoking.ID}/revocations`, { method: 'POST', headers: manager({ 'Content-Type': 'application/json' }), body: JSON.stringify({ reason }) }, tr('Revocado'))) setRevoking(null); }} />
  </div>;
}

// --- Certificados (OO-024/025/026) ---------------------------------------------------

type Eligible = { registration_id: string; display_name: string; ticket_id: string; attended_at: string; certificate_id: string | null };
type Certificate = { id: string; registration_id: string; display_name: string; verification_code: string; state: string; issued_at: string; revocation_reason?: string | null };

const certTone = (state: string): 'success' | 'warning' | 'danger' | 'neutral' => state === 'credential_issued' ? 'success' : state === 'revoked' ? 'danger' : state === 'replaced' || state === 'failed' ? 'warning' : 'neutral';

export function CertificatesPanel({ api }: { api: OnsiteApi }) {
  const [eligible, setEligible] = useState<Eligible[]>([]);
  const [certs, setCerts] = useState<Certificate[]>([]);
  const [action, setAction] = useState<{ kind: 'revoke' | 'reissue'; cert: Certificate } | null>(null);
  const [notice, setNotice] = useState<Notice>(null);
  const load = async () => {
    const [e, c] = await Promise.all([
      send(`${api.operations}/operations/v1/eligible-registrations?event_id=${encodeURIComponent(api.eventId)}`, { headers: api.headers() }).catch(() => null),
      send(`${api.operations}/operations/v1/certificates?event_id=${encodeURIComponent(api.eventId)}`, { headers: api.headers() }).catch(() => null),
    ]);
    if (e?.ok) setEligible(e.data.items || []);
    if (c?.ok) setCerts(c.data.items || []);
    if (!e?.ok || !c?.ok) setNotice({ tone: 'danger', text: onsiteReason((e?.data || c?.data)?.code) });
  };
  useEffect(() => { void load(); }, [api.eventId]);
  const post = async (url: string, body: unknown, ok: (data: any) => string, extra: Record<string, string> = {}) => {
    setNotice(null);
    const r = await send(url, { method: 'POST', headers: api.headers({ 'Content-Type': 'application/json', ...extra }), body: JSON.stringify(body) }).catch(() => null);
    if (!r?.ok) { setNotice({ tone: 'danger', text: onsiteReason(r?.data?.code) }); return false; }
    setNotice({ tone: 'success', text: ok(r.data) }); void load(); return true;
  };
  const verifyUrl = (code: string) => `${api.operations}/public/v1/certificates/${encodeURIComponent(code)}?format=html`;
  return <div className="nx-content-grid">
    <Card>
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">attendance@1.0.0</p><h2>{tr('Elegibles')}</h2><p className="nx-muted">{tr('Sólo aparece quien tiene una entrada vigente registrada.')}</p></div><Button variant="secondary" onClick={() => void load()}><RefreshCw size={16} />{tr('Actualizar')}</Button></div>
      {eligible.length === 0 ? <p className="nx-muted">{tr('Nadie cumple aún la regla de asistencia.')}</p> : <div className="nx-history-list">{eligible.map((e) => <div className="nx-history-row" key={e.registration_id}><div><strong>{e.display_name}</strong><span>{e.ticket_id.slice(0, 13)} · {new Date(e.attended_at).toLocaleString()}</span></div>{e.certificate_id ? <Badge tone="success">{tr('Ya tiene certificado')}</Badge> : <Button onClick={() => void post(`${api.operations}/operations/v1/certificate-jobs`, { event_id: api.eventId, registration_id: e.registration_id }, (d) => `${tr('Certificado emitido')}: ${d.verification_code}`)}><ShieldCheck size={16} />{tr('Emitir')}</Button>}</div>)}</div>}
      <NoticeLine notice={notice} />
    </Card>
    <Card>
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">HukID</p><h2>{tr('Emitidos')}</h2></div></div>
      {certs.length === 0 ? <p className="nx-muted">{tr('Aún no hay certificados.')}</p> : <div className="nx-history-list">{certs.map((c) => <div className="nx-history-row" key={c.id}><div><strong>{c.display_name}</strong><span>{c.verification_code} · {c.state}{c.revocation_reason ? ` · ${c.revocation_reason}` : ''}</span></div><div className="nx-actions"><Badge tone={certTone(c.state)}>{c.state}</Badge>
        {(c.state === 'eligible' || c.state === 'failed' || c.state === 'cancelled') && <Button variant="secondary" onClick={() => void post(`${api.operations}/operations/v1/certificates/${c.id}/hukid-deliveries`, { consent: true }, () => tr('Entrega HukID encolada.'), { 'Idempotency-Key': `nexo-cert-${c.id}-hukid-v1` })}>{tr('Enviar a HukID')}</Button>}
        {c.state !== 'revoked' && c.state !== 'replaced' && <><Button variant="ghost" onClick={() => setAction({ kind: 'reissue', cert: c })}>{tr('Reemitir')}</Button><Button variant="ghost" onClick={() => setAction({ kind: 'revoke', cert: c })}>{tr('Revocar')}</Button></>}
        <a className="nx-text-button" href={verifyUrl(c.verification_code)} target="_blank" rel="noreferrer" title={tr('Abrir verificación pública')}>{tr('Verificar')}</a>
      </div></div>)}</div>}
    </Card>
    <ReasonModal open={!!action} title={`${action?.kind === 'revoke' ? tr('Revocar') : tr('Reemitir')} · ${action?.cert.verification_code || ''}`} description={action?.cert.display_name || ''} onCancel={() => setAction(null)} onConfirm={async (reason) => { if (action && await post(`${api.operations}/operations/v1/certificates/${action.cert.id}/${action.kind === 'revoke' ? 'revocations' : 'reissues'}`, { reason }, (d) => action.kind === 'revoke' ? tr('Revocado') : `${tr('Certificado emitido')}: ${d.verification_code}`)) setAction(null); }} />
  </div>;
}

// --- Reportes (OO-020/021) ---------------------------------------------------------------

type Report = { id: string; report_type: string; format: string; status: string; progress: number; row_count: number | null; download_available: boolean; created_at: string; error_code: string | null };
const reportTypes = () => [{ value: 'attendance', label: tr('Asistencia') }, { value: 'presence', label: tr('Presentes ahora') }, { value: 'rejections', label: tr('Rechazos de QR') }, { value: 'summary', label: tr('Resumen') }];
const statusLabel: Record<string, string> = { pending: 'Pendiente', running: 'Generando', succeeded: 'Listo', failed: 'Falló', cancelled: 'Cancelado', expired: 'Vencido', purged: 'Purgado' };

export function ReportsPanel({ api }: { api: OnsiteApi }) {
  const [type, setType] = useState('attendance');
  const [format, setFormat] = useState('csv');
  const [items, setItems] = useState<Report[]>([]);
  const [notice, setNotice] = useState<Notice>(null);
  const [stats, setStats] = useState<Record<string, number>>({});
  const load = async () => {
    const r = await send(`${api.operations}/operations/v1/report-jobs?event_id=${encodeURIComponent(api.eventId)}`, { headers: api.headers() }).catch(() => null);
    if (r?.ok) setItems(r.data.items || []); else setNotice({ tone: 'danger', text: onsiteReason(r?.data?.code) });
    const s = await send(`${api.onsite}/onsite/v1/scan-stats?event_id=${encodeURIComponent(api.eventId)}`, { headers: api.headers({}, 'supervisor') }).catch(() => null);
    if (s?.ok) setStats(s.data.by_reason || {});
  };
  useEffect(() => { void load(); }, [api.eventId]);
  // Mientras haya reportes en curso, se consulta su avance.
  useEffect(() => {
    if (!items.some((i) => i.status === 'pending' || i.status === 'running')) return;
    const timer = window.setTimeout(() => void load(), 2000);
    return () => window.clearTimeout(timer);
  }, [items]);
  const post = async (url: string, body?: unknown) => {
    setNotice(null);
    const r = await send(url, { method: 'POST', headers: api.headers({ 'Content-Type': 'application/json' }), body: body ? JSON.stringify(body) : undefined }).catch(() => null);
    if (!r?.ok) setNotice({ tone: 'danger', text: onsiteReason(r?.data?.code) });
    void load();
  };
  const download = async (report: Report) => {
    const response = await fetch(`${api.operations}/operations/v1/report-jobs/${report.id}/artifact`, { headers: api.headers() }).catch(() => null);
    if (!response?.ok) { const data = await response?.json().catch(() => ({})); setNotice({ tone: 'danger', text: onsiteReason(data?.code) }); return; }
    const url = URL.createObjectURL(await response.blob());
    const link = document.createElement('a');
    link.href = url; link.download = `reporte-${report.report_type}-${api.eventId}.${report.format}`; link.click();
    URL.revokeObjectURL(url);
  };
  const typeName = (v: string) => reportTypes().find((t) => t.value === v)?.label || v;
  return <div className="nx-content-grid">
    <Card>
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">OO-020</p><h2>{tr('Reportes')}</h2></div><Button variant="secondary" onClick={() => void load()}><RefreshCw size={16} />{tr('Actualizar')}</Button></div>
      <div className="nx-ticket-entry"><SelectMenu label={tr('Tipo de reporte')} value={type} onChange={setType} options={reportTypes()} /><SelectMenu label={tr('Formato')} value={format} onChange={setFormat} options={[{ value: 'csv', label: 'CSV (Excel)' }, { value: 'json', label: 'JSON' }]} /><div className="nx-actions"><Button onClick={() => void post(`${api.operations}/operations/v1/report-jobs`, { event_id: api.eventId, scope: type === 'summary' ? '' : 'main', format, report_type: type })}><FileText size={16} />{tr('Generar reporte')}</Button></div></div>
      {items.length === 0 ? <p className="nx-muted">{tr('Aún no hay reportes.')}</p> : <div className="nx-history-list">{items.map((r) => <div className="nx-history-row" key={r.id}><div><strong>{typeName(r.report_type)} · {r.format.toUpperCase()}</strong><span>{new Date(r.created_at).toLocaleString()}{r.row_count !== null ? ` · ${r.row_count} ${tr('filas')}` : ''}{r.status === 'running' ? ` · ${r.progress}%` : ''}</span></div><div className="nx-actions"><Badge tone={r.status === 'succeeded' ? 'success' : r.status === 'failed' ? 'danger' : 'neutral'}>{tr(statusLabel[r.status] || r.status)}</Badge>
        {r.download_available && <Button variant="secondary" onClick={() => void download(r)}><Download size={16} />{tr('Descargar')}</Button>}
        {['failed', 'cancelled', 'expired', 'purged'].includes(r.status) && <Button variant="ghost" onClick={() => void post(`${api.operations}/operations/v1/report-jobs/${r.id}/retries`)}>{tr('Reintentar')}</Button>}
        {['succeeded', 'failed', 'cancelled', 'expired'].includes(r.status) && <Button variant="ghost" onClick={() => void post(`${api.operations}/operations/v1/report-jobs/${r.id}/purges`)}><Trash2 size={16} />{tr('Purgar')}</Button>}
      </div></div>)}</div>}
      <NoticeLine notice={notice} />
    </Card>
    <Card className="nx-readiness">
      <div className="nx-panel-heading"><div><p className="nx-eyebrow">OO-016</p><h2>{tr('Rechazos de QR (24 h)')}</h2></div></div>
      {Object.keys(stats).length === 0 ? <p className="nx-muted">{tr('Sin rechazos.')}</p> : Object.entries(stats).sort((a, b) => b[1] - a[1]).map(([reason, count]) => <StatusLine key={reason} label={onsiteReason(reason)} value={String(count)} tone="warning" />)}
    </Card>
  </div>;
}
