import type {Locale} from '../core/i18n';

/** Estado de un evento según sus fechas. */
export type EventStatus = 'pending' | 'inProgress' | 'finished';

/**
 * Evento tal como lo muestra la home. Las fechas van en ISO 8601, igual que las
 * entregaría la Public Read API (`GET /public/v1/sites/{slug}/events`) cuando exista.
 */
export type HomeEvent = {
  id: string;
  title: string;
  description: string;
  category: string;
  place: string;
  image: string;
  imageAlt: string;
  startsAt: string;
  endsAt: string;
  allDay: boolean;
};

/** Programación de un evento de demostración, relativa al día de hoy (como los seeds). */
export type DemoSchedule = {dayOffset: number; days: number; startHour?: number};

const DAY = 24 * 60 * 60 * 1000;

export function getEventStatus(startsAt: string, endsAt: string, now: Date = new Date()): EventStatus {
  const time = now.getTime();
  if (time < new Date(startsAt).getTime()) return 'pending';
  if (time > new Date(endsAt).getTime()) return 'finished';
  return 'inProgress';
}

/**
 * Milisegundos hasta que algún evento cambie de estado (empieza o termina), o `null` si ya
 * no queda ningún cambio por delante. Permite actualizar la home justo en ese momento.
 */
export function msUntilNextStatusChange(events: Pick<HomeEvent, 'startsAt' | 'endsAt'>[], now: Date = new Date()): number | null {
  const time = now.getTime();
  let next: number | null = null;
  for (const event of events) {
    // Empieza en `startsAt`; pasa a finalizado en cuanto se supera `endsAt`.
    for (const boundary of [new Date(event.startsAt).getTime(), new Date(event.endsAt).getTime() + 1]) {
      const wait = boundary - time;
      if (wait > 0 && (next === null || wait < next)) next = wait;
    }
  }
  return next;
}

/** En curso primero, luego los próximos (el más cercano antes) y al final los ya realizados (el más reciente antes). */
export function sortEventsForHome(events: HomeEvent[], now: Date = new Date()): HomeEvent[] {
  const rank: Record<EventStatus, number> = {inProgress: 0, pending: 1, finished: 2};
  return [...events].sort((a, b) => {
    const statusA = getEventStatus(a.startsAt, a.endsAt, now);
    const statusB = getEventStatus(b.startsAt, b.endsAt, now);
    if (statusA !== statusB) return rank[statusA] - rank[statusB];
    if (statusA === 'finished') return new Date(b.endsAt).getTime() - new Date(a.endsAt).getTime();
    return new Date(a.startsAt).getTime() - new Date(b.startsAt).getTime();
  });
}

/** Convierte una programación relativa en fechas reales a partir de hoy, en la zona horaria del visitante. */
export function scheduleFromToday({dayOffset, days, startHour}: DemoSchedule, now: Date = new Date()) {
  const today = new Date(now.getFullYear(), now.getMonth(), now.getDate());
  const start = new Date(today.getTime() + dayOffset * DAY);
  if (startHour !== undefined) start.setHours(startHour);
  const end = new Date(today.getTime() + (dayOffset + days) * DAY - 1);
  return {startsAt: start.toISOString(), endsAt: end.toISOString(), allDay: startHour === undefined};
}

const intlLocale: Record<Locale, string> = {es: 'es-PE', en: 'en-US'};

/** Fecha legible según el idioma: "24 nov 2026", "24 nov 2026, 11:00" o un rango como "14–16 nov 2026". */
export function formatEventDate(event: Pick<HomeEvent, 'startsAt' | 'endsAt' | 'allDay'>, locale: Locale): string {
  const start = new Date(event.startsAt);
  const end = new Date(event.endsAt);
  const date = new Intl.DateTimeFormat(intlLocale[locale], {day: 'numeric', month: 'short', year: 'numeric'});
  // Formato compacto y uniforme: sin puntos de abreviatura y, en español, sin los "de" del rango.
  const clean = (text: string) => text.replace(/\./g, '').replace(/ de /g, ' ');
  if (start.toDateString() !== end.toDateString()) return clean(date.formatRange(start, end));
  if (event.allDay) return clean(date.format(start));
  const time = new Intl.DateTimeFormat(intlLocale[locale], {hour: '2-digit', minute: '2-digit', hour12: false}).format(start);
  return `${clean(date.format(start))}, ${time}`;
}
