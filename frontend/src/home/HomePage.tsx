import {useMemo, useState} from 'react';
import type {ReactNode} from 'react';
import {Building2, CirclePlus, Compass, GraduationCap, Network, Users} from 'lucide-react';
import {useI18n} from '../core/i18n';
import {buildDemoEvents, homeContent} from './content';
import type {OfferIcon} from './content';
import {sortEventsForHome} from './events';
import {HomeNavbar} from './HomeNavbar';
import {HomeFooter} from './HomeFooter';
import {EventCarousel} from './EventCarousel';
import {useLiveNow} from './useLiveNow';
import './home.css';

const offerIcons: Record<OfferIcon, ReactNode> = {mentoring: <Users size={24}/>, training: <GraduationCap size={24}/>, networking: <Network size={24}/>, coworking: <Building2 size={24}/>};

export function HomePage({onAccess}: {onAccess: () => void}) {
  const {locale} = useI18n();
  const content = homeContent[locale];
  // Las fechas de la lista demo se fijan una sola vez al cargar; el reloj en vivo sólo decide el estado.
  // Con la API real, `baseEvents` vendrá del servidor y el estado seguirá calculándose igual.
  const [demoAnchor] = useState(() => new Date());
  const baseEvents = useMemo(() => buildDemoEvents(content.events.items, demoAnchor), [content, demoAnchor]);
  const now = useLiveNow(baseEvents);
  const events = useMemo(() => sortEventsForHome(baseEvents, now), [baseEvents, now]);
  return (
    <div className="nx-home">
      <a className="nx-skip-link" href="#contenido">{content.skipLink}</a>
      <HomeNavbar content={content.nav} onAccess={onAccess}/>
      <main id="contenido" className="nx-home-main">
        <section id="inicio" className="nx-home-hero">
          <div className="nx-home-container">
            <div className="nx-home-hero-pitch">
              <h1>{content.hero.titleLead} <span>{content.hero.titleHighlight}</span></h1>
              <p>{content.hero.description}</p>
              <div className="nx-home-hero-actions">
                <a className="nx-home-button nx-home-button-accent nx-home-button-lg" href="#eventos"><Compass size={18}/>{content.hero.explore}</a>
                <button type="button" className="nx-home-button nx-home-button-outline nx-home-button-lg" onClick={onAccess}><CirclePlus size={18}/>{content.hero.create}</button>
              </div>
            </div>
            <EventCarousel events={events} content={content.events} locale={locale} now={now} onAccess={onAccess}/>
          </div>
        </section>

        <section id="programas" className="nx-home-section nx-home-section-plain" aria-labelledby="home-offer-title">
          <div className="nx-home-container">
            <SectionHeading id="home-offer-title" title={content.offer.title} description={content.offer.description}/>
            <div className="nx-home-offer">
              {content.offer.items.map(item => (
                <article key={item.title} className="nx-home-card nx-home-offer-item">
                  <span className="nx-home-icon-tile">{offerIcons[item.icon]}</span>
                  <h3>{item.title}</h3>
                  <p>{item.description}</p>
                </article>
              ))}
            </div>
          </div>
        </section>
      </main>
      <HomeFooter content={content.footer}/>
    </div>
  );
}

function SectionHeading({id, title, description}: {id: string; title: string; description: string}) {
  return (
    <div className="nx-home-section-heading">
      <h2 id={id}>{title}</h2>
      <p>{description}</p>
    </div>
  );
}
