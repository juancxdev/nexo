import {act} from 'react';
import {createRoot} from 'react-dom/client';
import type {Root} from 'react-dom/client';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {HomePage} from './HomePage';

declare global { var IS_REACT_ACT_ENVIRONMENT: boolean; }

const statusesOf = (container: HTMLElement) => [...container.querySelectorAll('.nx-home-event-status')].map(status => status.textContent);

describe('home pública', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onAccess = vi.fn();

  const renderAt = (date: Date) => {
    vi.setSystemTime(date);
    act(() => root.render(<HomePage onAccess={onAccess}/>));
  };

  beforeEach(() => {
    vi.useFakeTimers({toFake: ['Date', 'setTimeout', 'clearTimeout']});
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    localStorage.setItem('nexo.locale', 'es');
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    renderAt(new Date(2026, 10, 24, 12, 0));
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    onAccess.mockReset();
    vi.useRealTimers();
  });

  it('el navbar sólo tiene el logo de Nexo y el botón de acceso', () => {
    const nav = container.querySelector('header nav')!;
    const links = nav.querySelectorAll('a');
    const buttons = nav.querySelectorAll('button');
    expect(links).toHaveLength(1);
    expect(links[0].textContent).toContain('Nexo');
    expect(buttons).toHaveLength(1);
    expect(buttons[0].textContent).toBe('Acceder');
  });

  it('el botón de acceso lleva al login', () => {
    act(() => container.querySelector<HTMLButtonElement>('header nav button')!.click());
    expect(onAccess).toHaveBeenCalledTimes(1);
  });

  it('el footer muestra el nombre de la app y los derechos reservados', () => {
    const footer = container.querySelector('footer')!;
    expect(footer.textContent).toContain('NEXO');
    expect(footer.textContent).toContain('Todos los derechos reservados.');
  });

  it('muestra las secciones del diseño', () => {
    const titles = [...container.querySelectorAll('h1, h2')].map(heading => heading.textContent);
    expect(titles).toEqual([
      'Conéctate, Suscríbete y Crea Eventos Extraordinarios',
      'Lo que Ofrecemos para Asistentes y Creadores',
    ]);
    expect(container.querySelectorAll('.nx-home-event')).toHaveLength(6);
    expect(container.querySelectorAll('.nx-home-offer-item')).toHaveLength(4);
  });

  it('no incluye cifras, premios ni aliados', () => {
    const text = container.textContent!;
    expect(text).not.toContain('Premios y Reconocimientos');
    expect(text).not.toContain('Nuestros Aliados');
    expect(text).not.toContain('Capital semilla');
    expect(container.querySelector('.nx-home-metric, .nx-home-award, .nx-home-allies')).toBeNull();
  });

  it('cada evento muestra su estado y aparecen primero los que están en curso', () => {
    expect(statusesOf(container)).toEqual(['En proceso', 'Pendiente', 'Pendiente', 'Pendiente', 'Finalizado', 'Finalizado']);
    expect(container.querySelector('.nx-home-event h3')!.textContent).toBe('Demo Day IncubaUNAS');
    expect(container.querySelectorAll('.nx-home-status-dot')).toHaveLength(6);
  });

  it('los eventos finalizados invitan a ver detalles en lugar de registrarse', () => {
    const finished = container.querySelector('.nx-home-event.is-finished')!;
    expect(finished.querySelector('.nx-home-link')!.textContent).toBe('Ver detalles');
    expect(container.querySelector('.nx-home-event.is-pending .nx-home-link')!.textContent).toBe('Registrarse');
  });

  it('"Crea el tuyo" lleva al login', () => {
    const link = container.querySelector<HTMLButtonElement>('.nx-home-create-link')!;
    expect(link.textContent).toBe('Crea el tuyo');
    act(() => link.click());
    expect(onAccess).toHaveBeenCalledTimes(1);
  });

  it('el estado cambia en tiempo real cuando el evento termina, sin recargar', () => {
    act(() => root.unmount());
    root = createRoot(container);
    renderAt(new Date(2026, 10, 24, 23, 59, 30));
    const demoDay = () => [...container.querySelectorAll('.nx-home-event')].find(card => card.querySelector('h3')!.textContent === 'Demo Day IncubaUNAS')!;
    expect(demoDay().querySelector('.nx-home-event-status')!.textContent).toBe('En proceso');

    act(() => vi.advanceTimersByTime(40_000));

    expect(demoDay().querySelector('.nx-home-event-status')!.textContent).toBe('Finalizado');
    expect(demoDay().querySelector('.nx-home-link')!.textContent).toBe('Ver detalles');
    expect(statusesOf(container)).toEqual(['Pendiente', 'Pendiente', 'Pendiente', 'Finalizado', 'Finalizado', 'Finalizado']);
  });

  it('un evento pendiente pasa a "En proceso" justo cuando empieza, también al volver a la pestaña', () => {
    // Cargada el 24 a mediodía, la Masterclass de la lista demo empieza el 28 a las 10:00.
    const masterclass = () => [...container.querySelectorAll('.nx-home-event')].find(card => card.querySelector('h3')!.textContent!.startsWith('Masterclass'))!;
    expect(masterclass().querySelector('.nx-home-event-status')!.textContent).toBe('Pendiente');

    // La pestaña quedó abierta días; al volver, la home se re-sincroniza con la hora real.
    vi.setSystemTime(new Date(2026, 10, 28, 9, 59, 50));
    act(() => window.dispatchEvent(new Event('focus')));
    expect(masterclass().querySelector('.nx-home-event-status')!.textContent).toBe('Pendiente');

    act(() => vi.advanceTimersByTime(11_000));
    expect(masterclass().querySelector('.nx-home-event-status')!.textContent).toBe('En proceso');
  });

  it('las flechas deslizan el carrusel y se desactivan en los extremos', () => {
    const track = container.querySelector<HTMLDivElement>('#home-events-track')!;
    const [previous, next] = container.querySelectorAll<HTMLButtonElement>('.nx-home-arrow');
    expect(previous.getAttribute('aria-label')).toBe('Ver eventos anteriores');
    expect(previous.disabled).toBe(true);

    Object.defineProperty(track, 'scrollWidth', {configurable: true, value: 2000});
    Object.defineProperty(track, 'clientWidth', {configurable: true, value: 1000});
    Object.defineProperty(track.querySelector('[data-carousel-item]')!, 'offsetWidth', {configurable: true, value: 320});
    track.scrollBy = vi.fn();
    act(() => track.dispatchEvent(new Event('scroll')));
    expect(next.disabled).toBe(false);

    act(() => next.click());
    expect(track.scrollBy).toHaveBeenCalledWith({left: 320, behavior: 'smooth'});

    Object.defineProperty(track, 'scrollLeft', {configurable: true, value: 1000});
    act(() => track.dispatchEvent(new Event('scroll')));
    expect(next.disabled).toBe(true);
    expect(previous.disabled).toBe(false);
  });
});
