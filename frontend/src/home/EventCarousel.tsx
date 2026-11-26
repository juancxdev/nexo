import {useCallback, useEffect, useRef, useState} from 'react';
import {ArrowRight, CalendarDays, ChevronLeft, ChevronRight, MapPin} from 'lucide-react';
import type {Locale} from '../core/i18n';
import {formatEventDate, getEventStatus} from './events';
import type {HomeEvent} from './events';
import type {HomeContent} from './content';

type EventCarouselProps = {
  events: HomeEvent[];
  content: HomeContent['events'];
  locale: Locale;
  now: Date;
  onAccess: () => void;
  onSelectEvent?: (event: HomeEvent) => void;
};

/** Carrusel horizontal de eventos: flechas, deslizamiento táctil y teclado (el riel recibe foco). */
export function EventCarousel({events, content, locale, now, onAccess, onSelectEvent}: EventCarouselProps) {
  const trackRef = useRef<HTMLDivElement>(null);
  const [canPrev, setCanPrev] = useState(false);
  const [canNext, setCanNext] = useState(false);

  const updateArrows = useCallback(() => {
    const track = trackRef.current;
    if (!track) return;
    setCanPrev(track.scrollLeft > 4);
    setCanNext(track.scrollLeft + track.clientWidth < track.scrollWidth - 4);
  }, []);

  useEffect(() => {
    const track = trackRef.current;
    updateArrows();
    track?.addEventListener('scroll', updateArrows, {passive: true});
    window.addEventListener('resize', updateArrows);
    return () => {
      track?.removeEventListener('scroll', updateArrows);
      window.removeEventListener('resize', updateArrows);
    };
  }, [events.length, updateArrows]);

  const slide = (direction: 1 | -1) => {
    const track = trackRef.current;
    if (!track) return;
    const card = track.querySelector<HTMLElement>('[data-carousel-item]');
    const gap = parseFloat(getComputedStyle(track).columnGap) || 0;
    const step = card ? card.offsetWidth + gap : track.clientWidth;
    const reduceMotion = window.matchMedia?.('(prefers-reduced-motion: reduce)').matches;
    track.scrollBy({left: direction * step, behavior: reduceMotion ? 'auto' : 'smooth'});
  };

  return (
    <div id="eventos" className="nx-home-carousel">
      <div className="nx-home-events-toolbar">
        <button type="button" className="nx-home-create-link" onClick={onAccess}>
          {content.createYours}<ArrowRight size={14}/>
        </button>
        <div className="nx-home-carousel-controls">
          <button type="button" className="nx-home-arrow" aria-label={content.previous} aria-controls="home-events-track" disabled={!canPrev} onClick={() => slide(-1)}>
            <ChevronLeft size={20}/>
          </button>
          <button type="button" className="nx-home-arrow" aria-label={content.next} aria-controls="home-events-track" disabled={!canNext} onClick={() => slide(1)}>
            <ChevronRight size={20}/>
          </button>
        </div>
      </div>
      <div id="home-events-track" ref={trackRef} className="nx-home-events" role="list" aria-label={content.label} tabIndex={0}>
        {events.map(event => {
          const status = getEventStatus(event.startsAt, event.endsAt, now);
          return (
            <article key={event.id} className={`nx-home-card nx-home-event is-${status}`} role="listitem" data-carousel-item>
              <div className="nx-home-event-media">
                <img src={event.image} alt={event.imageAlt} loading="lazy"/>
                <span className={`nx-home-event-status nx-home-status-${status}`}>
                  <span className="nx-home-status-dot" aria-hidden="true"/>
                  {content.status[status]}
                </span>
                <span className="nx-home-event-category">{event.category}</span>
              </div>
              <div className="nx-home-event-body">
                <p className="nx-home-event-date">
                  <CalendarDays size={16}/>
                  <time dateTime={event.startsAt}>{formatEventDate(event, locale)}</time>
                </p>
                <h3>{event.title}</h3>
                <p className="nx-home-event-description">{event.description}</p>
                <div className="nx-home-event-footer">
                  <span className="nx-home-event-place"><MapPin size={16}/>{event.place}</span>
                  <button type="button" className="nx-home-link" onClick={() => (event.slug && onSelectEvent ? onSelectEvent(event) : onAccess())}>
                    {status === 'finished' ? content.details : content.register}<ArrowRight size={16}/>
                  </button>
                </div>
              </div>
            </article>
          );
        })}
      </div>
    </div>
  );
}
