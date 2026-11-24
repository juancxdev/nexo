import {describe, expect, it} from 'vitest';
import {formatEventDate, getEventStatus, msUntilNextStatusChange, scheduleFromToday, sortEventsForHome} from './events';
import type {HomeEvent} from './events';

const now = new Date(2026, 10, 24, 12, 0);
const iso = (y: number, m: number, d: number, h = 0, min = 0) => new Date(y, m, d, h, min).toISOString();

describe('estado de un evento', () => {
  it('es pendiente antes de empezar, en proceso mientras ocurre y finalizado al terminar', () => {
    expect(getEventStatus(iso(2026, 10, 25), iso(2026, 10, 26), now)).toBe('pending');
    expect(getEventStatus(iso(2026, 10, 24, 9), iso(2026, 10, 24, 18), now)).toBe('inProgress');
    expect(getEventStatus(iso(2026, 10, 20), iso(2026, 10, 21), now)).toBe('finished');
  });

  it('cuenta los límites exactos como en proceso', () => {
    expect(getEventStatus(now.toISOString(), iso(2026, 10, 24, 18), now)).toBe('inProgress');
    expect(getEventStatus(iso(2026, 10, 24, 9), now.toISOString(), now)).toBe('inProgress');
  });
});

describe('próximo cambio de estado', () => {
  it('devuelve cuánto falta para el inicio o fin más cercano', () => {
    const events = [
      {startsAt: iso(2026, 10, 24, 9), endsAt: iso(2026, 10, 24, 12, 30)},
      {startsAt: iso(2026, 10, 24, 14), endsAt: iso(2026, 10, 24, 18)},
    ];
    expect(msUntilNextStatusChange(events, now)).toBe(30 * 60 * 1000 + 1);
  });

  it('devuelve null cuando todos los eventos ya terminaron', () => {
    expect(msUntilNextStatusChange([{startsAt: iso(2026, 9, 1), endsAt: iso(2026, 9, 2)}], now)).toBeNull();
  });
});

describe('orden de la home', () => {
  const event = (id: string, startsAt: string, endsAt: string): HomeEvent => ({id, title: id, description: '', category: '', place: '', image: '', imageAlt: '', startsAt, endsAt, allDay: true});

  it('muestra en curso, luego próximos del más cercano y al final finalizados del más reciente', () => {
    const sorted = sortEventsForHome([
      event('viejo', iso(2026, 9, 1), iso(2026, 9, 2)),
      event('lejano', iso(2026, 11, 20), iso(2026, 11, 21)),
      event('hoy', iso(2026, 10, 24), iso(2026, 10, 24, 23)),
      event('reciente', iso(2026, 10, 10), iso(2026, 10, 11)),
      event('cercano', iso(2026, 10, 28), iso(2026, 10, 28, 23)),
    ], now);
    expect(sorted.map(item => item.id)).toEqual(['hoy', 'cercano', 'lejano', 'reciente', 'viejo']);
  });
});

describe('fechas relativas y formato', () => {
  it('programa eventos a partir de hoy, de día completo o con hora', () => {
    const allDay = scheduleFromToday({dayOffset: 0, days: 1}, now);
    expect(getEventStatus(allDay.startsAt, allDay.endsAt, now)).toBe('inProgress');
    expect(allDay.allDay).toBe(true);
    const withHour = scheduleFromToday({dayOffset: 4, days: 1, startHour: 10}, now);
    expect(new Date(withHour.startsAt)).toEqual(new Date(2026, 10, 28, 10));
    expect(withHour.allDay).toBe(false);
  });

  it('muestra un día, un día con hora o un rango según el idioma', () => {
    expect(formatEventDate({startsAt: iso(2026, 10, 24), endsAt: iso(2026, 10, 24, 23, 59), allDay: true}, 'es')).toMatch(/24 nov 2026/);
    expect(formatEventDate({startsAt: iso(2026, 11, 24, 11), endsAt: iso(2026, 11, 24, 23, 59), allDay: false}, 'es')).toMatch(/24 dic 2026, 11:00/);
    expect(formatEventDate({startsAt: iso(2026, 11, 5), endsAt: iso(2026, 11, 6, 23, 59), allDay: true}, 'es')).toMatch(/^5\s*–\s*6 dic 2026$/);
    expect(formatEventDate({startsAt: iso(2026, 11, 5), endsAt: iso(2026, 11, 6, 23, 59), allDay: true}, 'en')).toMatch(/Dec 5\s*–\s*6, 2026/);
  });
});
