import { useEffect, useMemo, useRef, useState } from 'react';
import { ArrowRight, CheckCircle2, Ticket, Users } from 'lucide-react';
import QRCode from 'qrcode';
import { Alert, Badge, Button, Card, Input, PageHeading } from '../../design-system';
import { useI18n } from '../../core/i18n';
import type { ModuleContext } from '../../platform/microfrontends/contracts';
import { createParticipationClient, type HukidCredentialDelivery, type ParticipationForm, type RegistrationReceipt, type TicketResponse } from './api';
import { solveParticipationChallenge } from './proof-of-work';
import './participation.css';

const defaultSite = import.meta.env.VITE_PARTICIPATION_SITE_SLUG || 'incuba-unas';
const defaultEvent = import.meta.env.VITE_PARTICIPATION_EVENT_SLUG || 'congreso-innovacion-demo-day-2026';

export function ParticipationModule({ context }: { context: ModuleContext }) {
  const { t, locale } = useI18n();
  const params = useMemo(() => new URLSearchParams(window.location.search), []);
  const site = params.get('site') || defaultSite;
  const event = params.get('event') || defaultEvent;
  const api = useMemo(() => createParticipationClient(context.apiBaseUrl), [context.apiBaseUrl]);
  const [form, setForm] = useState<ParticipationForm | null>(null);
  const [receipt, setReceipt] = useState<RegistrationReceipt | null>(null);
  const [ticket, setTicket] = useState<TicketResponse | null>(null);
  const [credential, setCredential] = useState<HukidCredentialDelivery | null>(null);
  const [values, setValues] = useState({ fullName: '', email: '', institution: '' });
  const [answers, setAnswers] = useState<Record<string, string | number | boolean>>({});
  const idempotencyKey = useRef(`participation-${crypto.randomUUID()}`);
  const [loading, setLoading] = useState(true);
  const [submitting, setSubmitting] = useState(false);
  const [error, setError] = useState('');

  useEffect(() => {
    let active = true;
    setLoading(true);
    api.loadFormV2(site, event).then((result) => { if (active) setForm(result); })
      .catch((reason: unknown) => { if (active) setError(reason instanceof Error ? reason.message : (locale === 'en' ? 'The event form could not be loaded.' : 'No se pudo cargar el formulario del evento.')); })
      .finally(() => { if (active) setLoading(false); });
    return () => { active = false; };
  }, [api, locale]);

  const update = (field: keyof typeof values) => (eventValue: string) => setValues((current) => ({ ...current, [field]: eventValue }));
  const submit = async (eventValue: React.FormEvent) => {
    eventValue.preventDefault();
    setError('');
    setSubmitting(true);
    try {
      const challenge = await api.getChallenge(site, event);
      const proof = await solveParticipationChallenge(challenge.challenge, challenge.difficulty);
      const result = await api.registerV2(site, event, {
        form_version_id: form?.form_version_id,
        consent_version: form?.consent_version,
        consent_accepted: true,
        attendee: { full_name: values.fullName, email: values.email, institution: values.institution },
        answers,
      }, idempotencyKey.current, challenge.challenge, proof);
      setReceipt(result);
      if (result.state === 'confirmed' && result.self_access_token) {
        setTicket(await api.getTicket(result.id, result.self_access_token));
        for (let attempt = 0; attempt < 12; attempt += 1) {
          const delivery = await api.getCredential(result.id, result.self_access_token).catch(() => null);
          if (delivery) {
            setCredential(delivery);
            if (delivery.credential_offer_uri || ['credential_issued', 'failed', 'cancelled'].includes(delivery.state)) break;
          }
          await new Promise((resolve) => setTimeout(resolve, 1000));
        }
      }
    } catch (reason: unknown) {
      setError(reason instanceof Error ? reason.message : t('participation.submitError'));
    } finally { setSubmitting(false); }
  };

  if (loading) return <Card className="nx-participation-state"><p>{t('participation.loading')}</p></Card>;
  if (receipt) return <ParticipationReceipt receipt={receipt} ticket={ticket} credential={credential} />;

  const isOpen = form?.binding_state === 'open';
  const answerProperties = Object.entries(form?.schema?.properties || {}).filter(([name]) => !['full_name', 'email', 'institution'].includes(name));
  return <div className="nx-participation-page">
    <PageHeading eyebrow={t('participation.eyebrow')} title={form?.event_title || t('participation.title')} description={t('participation.description')} />
    <div className="nx-participation-grid">
      <Card className="nx-participation-card">
        <div className="nx-participation-card-heading"><div><span className="nx-eyebrow">{t('participation.step')}</span><h2>{t('participation.formTitle')}</h2></div><Badge tone={isOpen ? 'success' : 'warning'}>{isOpen ? t('participation.open') : t('participation.closed')}</Badge></div>
        {error && <Alert tone="danger">{error}</Alert>}
        {!form || !isOpen ? <div className="nx-participation-empty"><Ticket size={28} aria-hidden="true" /><p>{t('participation.unavailable')}</p></div> : <form className="nx-participation-form" onSubmit={submit}>
          <Input label={t('participation.fullName')} value={values.fullName} onChange={(eventValue) => update('fullName')(eventValue.target.value)} required autoComplete="name" />
          <Input label={t('participation.email')} type="email" value={values.email} onChange={(eventValue) => update('email')(eventValue.target.value)} required autoComplete="email" />
          <Input label={t('participation.institution')} value={values.institution} onChange={(eventValue) => update('institution')(eventValue.target.value)} autoComplete="organization" />
          {answerProperties.map(([name, definition]) => <Input key={name} label={definition.title || name} type={definition.type === 'number' ? 'number' : 'text'} value={String(answers[name] ?? '')} onChange={(eventValue) => setAnswers((current) => ({ ...current, [name]: definition.type === 'number' ? Number(eventValue.target.value) : eventValue.target.value }))} required />)}
          <label className="nx-participation-consent"><input type="checkbox" required defaultChecked /><span>{form.consent_text}</span></label>
          <Button type="submit" loading={submitting}>{submitting ? t('participation.submitting') : t('participation.submit')} <ArrowRight size={16} aria-hidden="true" /></Button>
        </form>}
      </Card>
      <Card className="nx-participation-aside"><Users size={24} aria-hidden="true" /><h2>{t('participation.whyTitle')}</h2><p>{t('participation.whyDescription')}</p>{form?.capacity_summary?.remaining !== undefined && <div className="nx-participation-capacity"><strong>{form.capacity_summary.remaining}</strong><span>{t('participation.remaining')}</span></div>}</Card>
    </div>
  </div>;
}

function ParticipationReceipt({ receipt, ticket, credential }: { receipt: RegistrationReceipt; ticket: TicketResponse | null; credential: HukidCredentialDelivery | null }) {
  const { t } = useI18n();
  const status = receipt.state === 'confirmed' ? t('participation.confirmed') : receipt.state === 'waitlisted' ? t('participation.waitlisted') : t('participation.pending');
  return <div className="nx-participation-receipt"><Card><div className="nx-participation-success"><CheckCircle2 size={36} aria-hidden="true" /><span className="nx-eyebrow">{t('participation.receiptEyebrow')}</span><h1>{t('participation.receiptTitle')}</h1><p>{t('participation.receiptDescription')}</p><Badge tone="success">{status}</Badge></div><div className="nx-participation-receipt-details"><span>{t('participation.receiptId')}</span><strong>{receipt.id}</strong>{receipt.waitlist_position && <span>{t('participation.waitlistPosition', { position: String(receipt.waitlist_position) })}</span>}</div>{credential?.credential_offer_uri ? <div className="nx-participation-ticket"><Ticket size={24} aria-hidden="true" /><div><strong>{t('participation.credentialAvailable')}</strong><span>{t('participation.credentialInstructions')}</span><TicketQr jws={credential.credential_offer_uri}/></div></div> : <div className="nx-participation-ticket"><Ticket size={24} aria-hidden="true" /><div><strong>{t('participation.credentialPreparing')}</strong><span>{t('participation.credentialPreparingDescription')}</span></div></div>}</Card></div>;
}

function TicketQr({ jws }: { jws: string }) {
  const { t } = useI18n();
  const [source, setSource] = useState('');
  useEffect(() => { let active = true; QRCode.toDataURL(jws, { margin: 1, width: 180 }).then((value) => { if (active) setSource(value); }); return () => { active = false; }; }, [jws]);
  if (!source) return <span>{t('participation.ticketPreparing')}</span>;
  return <a className="nx-participation-qr" href={source} download="nexo-ticket-qr.png"><img src={source} alt={t('participation.ticketQrAlt')} /><span>{t('participation.ticketDownload')}</span></a>;
}
