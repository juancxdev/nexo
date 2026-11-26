import {useEffect, useState} from 'react';
import {msUntilNextStatusChange} from './events';
import type {HomeEvent} from './events';

/** Revisión de seguridad aunque no haya cambios a la vista (reloj del equipo ajustado, suspensión, etc.). */
export const LIVE_REFRESH_MAX_MS = 60_000;
/** Pequeño margen para despertar justo después del instante del cambio. */
const BOUNDARY_MARGIN_MS = 50;

/**
 * Hora actual que se actualiza sola en el instante en que algún evento empieza o termina,
 * como mínimo una vez por minuto y al volver a la pestaña. Así el estado de cada evento
 * (pendiente, en proceso, finalizado) cambia en tiempo real sin recargar la página.
 */
export function useLiveNow(events: Pick<HomeEvent, 'startsAt' | 'endsAt'>[]): Date {
  const [now, setNow] = useState(() => new Date());

  useEffect(() => {
    const untilChange = msUntilNextStatusChange(events, now);
    const wait = untilChange === null ? LIVE_REFRESH_MAX_MS : Math.min(untilChange + BOUNDARY_MARGIN_MS, LIVE_REFRESH_MAX_MS);
    const timer = window.setTimeout(() => setNow(new Date()), wait);
    return () => window.clearTimeout(timer);
  }, [events, now]);

  useEffect(() => {
    // Al volver a la pestaña o a la ventana, los temporizadores pudieron quedar en pausa: se re-sincroniza.
    const refresh = () => setNow(new Date());
    const refreshIfVisible = () => {
      if (document.visibilityState === 'visible') refresh();
    };
    document.addEventListener('visibilitychange', refreshIfVisible);
    window.addEventListener('focus', refresh);
    return () => {
      document.removeEventListener('visibilitychange', refreshIfVisible);
      window.removeEventListener('focus', refresh);
    };
  }, []);

  return now;
}
