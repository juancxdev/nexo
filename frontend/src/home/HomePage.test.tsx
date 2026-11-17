import {act} from 'react';
import {createRoot} from 'react-dom/client';
import type {Root} from 'react-dom/client';
import {afterEach, beforeEach, describe, expect, it, vi} from 'vitest';
import {HomePage} from './HomePage';

declare global { var IS_REACT_ACT_ENVIRONMENT: boolean; }

describe('home pública', () => {
  let container: HTMLDivElement;
  let root: Root;
  const onAccess = vi.fn();

  beforeEach(() => {
    globalThis.IS_REACT_ACT_ENVIRONMENT = true;
    localStorage.setItem('nexo.locale', 'es');
    container = document.createElement('div');
    document.body.appendChild(container);
    root = createRoot(container);
    act(() => root.render(<HomePage onAccess={onAccess}/>));
  });

  afterEach(() => {
    act(() => root.unmount());
    container.remove();
    onAccess.mockReset();
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
      'Premios y Reconocimientos',
      'Lo que Ofrecemos para Asistentes y Creadores',
      'Nuestros Aliados',
    ]);
    expect(container.querySelectorAll('.nx-home-event')).toHaveLength(3);
    expect(container.querySelectorAll('.nx-home-metric')).toHaveLength(8);
  });
});
