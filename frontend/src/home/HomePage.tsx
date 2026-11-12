import type {ReactNode} from 'react';
import {ArrowRight, Award, Building2, CalendarDays, CircleCheck, CirclePlus, Compass, GraduationCap, MapPin, Medal, Network, Sparkles, Trophy, Users} from 'lucide-react';
import {useI18n} from '../core/i18n';
import {homeContent} from './content';
import type {AwardIcon, EventStatus, HomeEvent, OfferIcon} from './content';
import {homeImages} from './images';
import {HomeNavbar} from './HomeNavbar';
import {HomeFooter} from './HomeFooter';
import './home.css';

const awardIcons: Record<AwardIcon, ReactNode> = {trophy: <Trophy size={24}/>, award: <Award size={24}/>, medal: <Medal size={24}/>};
const offerIcons: Record<OfferIcon, ReactNode> = {mentoring: <Users size={24}/>, training: <GraduationCap size={24}/>, networking: <Network size={24}/>, coworking: <Building2 size={24}/>};
const statusIcons: Record<EventStatus, ReactNode> = {live: <span className="nx-home-live-dot" aria-hidden="true"/>, open: <CircleCheck size={13}/>, lastSeats: <Sparkles size={13}/>};

export function HomePage({onAccess}: {onAccess: () => void}) {
  const {locale} = useI18n();
  const content = homeContent[locale];
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
            <div id="eventos" className="nx-home-events" role="list" aria-label={content.events.label}>
              {content.events.items.map(event => <EventCard key={event.id} event={event} registerLabel={content.events.register} onRegister={onAccess}/>)}
            </div>
          </div>
        </section>

        <section className="nx-home-section nx-home-section-plain" aria-label={content.metrics.label}>
          <div className="nx-home-container nx-home-metrics">
            {content.metrics.items.map(metric => (
              <div key={metric.label} className="nx-home-metric">
                <p className={`nx-home-metric-value nx-home-tone-${metric.tone}`}>{metric.value}</p>
                <h3>{metric.label}</h3>
              </div>
            ))}
          </div>
        </section>

        <section className="nx-home-section nx-home-section-tinted" aria-labelledby="home-awards-title">
          <div className="nx-home-container">
            <SectionHeading id="home-awards-title" title={content.awards.title} description={content.awards.description}/>
            <div className="nx-home-awards">
              {content.awards.items.map((award, index) => (
                <article key={award.title} className="nx-home-card nx-home-award">
                  <span className={`nx-home-icon-tile ${index === 1 ? 'nx-home-icon-tile-accent' : ''}`}>{awardIcons[award.icon]}</span>
                  <h3>{award.title}</h3>
                </article>
              ))}
            </div>
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

        <section className="nx-home-section nx-home-section-tinted" aria-labelledby="home-allies-title">
          <div className="nx-home-container">
            <SectionHeading id="home-allies-title" title={content.allies.title} description={content.allies.description}/>
            <div className="nx-home-card nx-home-allies">
              <img src={homeImages.allies} alt={content.allies.imageAlt} loading="lazy"/>
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

function EventCard({event, registerLabel, onRegister}: {event: HomeEvent; registerLabel: string; onRegister: () => void}) {
  return (
    <article className="nx-home-card nx-home-event" role="listitem">
      <div className="nx-home-event-media">
        <img src={event.image} alt={event.imageAlt} loading="lazy"/>
        <span className="nx-home-event-status">{statusIcons[event.status]}{event.statusLabel}</span>
        <span className="nx-home-event-category">{event.category}</span>
      </div>
      <div className="nx-home-event-body">
        <p className="nx-home-event-date"><CalendarDays size={16}/>{event.date}</p>
        <h3>{event.title}</h3>
        <p className="nx-home-event-description">{event.description}</p>
        <div className="nx-home-event-footer">
          <span className="nx-home-event-place"><MapPin size={16}/>{event.place}</span>
          <button type="button" className="nx-home-link" onClick={onRegister}>{registerLabel}<ArrowRight size={16}/></button>
        </div>
      </div>
    </article>
  );
}
