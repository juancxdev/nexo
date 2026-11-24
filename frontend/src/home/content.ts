import type {Locale} from '../core/i18n';
import {homeImages} from './images';
import {scheduleFromToday} from './events';
import type {DemoSchedule, EventStatus, HomeEvent} from './events';

/** Textos de un evento de demostración; sus fechas salen de `demoEventSchedule`. */
export type HomeEventText = Pick<HomeEvent, 'id' | 'title' | 'description' | 'category' | 'place' | 'imageAlt'>;
export type OfferIcon = 'mentoring' | 'training' | 'networking' | 'coworking';

export type HomeContent = {
  skipLink: string;
  nav: {label: string; logoLabel: string; access: string};
  hero: {titleLead: string; titleHighlight: string; description: string; explore: string; create: string};
  events: {
    label: string;
    register: string;
    details: string;
    createYours: string;
    previous: string;
    next: string;
    status: Record<EventStatus, string>;
    items: HomeEventText[];
  };
  offer: {title: string; description: string; items: {icon: OfferIcon; title: string; description: string}[]};
  footer: {rights: string};
};

/**
 * Lista de demostración mientras no exista la Public Read API de eventos.
 * Las fechas son relativas a hoy (igual que los seeds de Onsite y Eventos), así siempre
 * hay eventos en los tres estados. Los nombres de Demo Day y del Encuentro vienen de esos seeds.
 */
export const demoEventSchedule: Record<string, DemoSchedule & {image: string}> = {
  'demo-day': {dayOffset: 0, days: 1, image: homeImages.techSummit},
  'masterclass': {dayOffset: 4, days: 1, startHour: 10, image: homeImages.masterclass},
  'product-forum': {dayOffset: 11, days: 2, image: homeImages.productForum},
  'encuentro-innovacion': {dayOffset: 30, days: 1, startHour: 11, image: homeImages.masterclass},
  'tech-summit': {dayOffset: -10, days: 3, image: homeImages.techSummit},
  'biostartups': {dayOffset: -25, days: 2, image: homeImages.productForum},
};

export function buildDemoEvents(items: HomeEventText[], now: Date = new Date()): HomeEvent[] {
  return items.map(item => {
    const {image, ...schedule} = demoEventSchedule[item.id];
    return {...item, image, ...scheduleFromToday(schedule, now)};
  });
}

export const homeContent: Record<Locale, HomeContent> = {
  es: {
    skipLink: 'Saltar al contenido',
    nav: {label: 'Navegación principal', logoLabel: 'Nexo, volver al inicio', access: 'Acceder'},
    hero: {
      titleLead: 'Conéctate, Suscríbete y Crea',
      titleHighlight: 'Eventos Extraordinarios',
      description: 'Somos la incubadora de la Universidad Nacional Agraria de la Selva. Brindamos asesoría y asistencia técnica para la creación de emprendimientos innovadores.',
      explore: 'Explorar eventos',
      create: 'Crear mi evento',
    },
    events: {
      label: 'Eventos destacados',
      register: 'Registrarse',
      details: 'Ver detalles',
      createYours: 'Crea el tuyo',
      previous: 'Ver eventos anteriores',
      next: 'Ver más eventos',
      status: {pending: 'Pendiente', inProgress: 'En proceso', finished: 'Finalizado'},
      items: [
        {id: 'demo-day', category: 'Emprendimiento', title: 'Demo Day IncubaUNAS', description: 'Las startups incubadas presentan sus proyectos ante mentores, inversionistas y aliados del ecosistema.', place: 'Auditorio UNAS', imageAlt: 'Auditorio con público frente a un escenario con pantallas'},
        {id: 'masterclass', category: 'Masterclass', title: 'Masterclass de Innovación y Venture Capital', description: 'Aprende estrategias de levantamiento de capital institucional, estructuración de rondas seed y escalamiento acelerado con socios inversores.', place: 'Campus Central & Online', imageAlt: 'Taller colaborativo con personas trabajando en mesas con laptops'},
        {id: 'product-forum', category: 'Diseño', title: 'Product Experience Forum & Expo', description: 'Un encuentro exclusivo para diseñadores de experiencia, líderes de producto y directores creativos enfocados en interfaces de próxima generación.', place: 'Hub de Innovación', imageAlt: 'Exposición de diseño de producto con prototipos interactivos'},
        {id: 'encuentro-innovacion', category: 'Innovación', title: 'Encuentro de innovación', description: 'Una jornada para conectar emprendedores, investigadores y empresas alrededor de proyectos con impacto en la región.', place: 'Pabellón de Innovación', imageAlt: 'Personas conversando en mesas de trabajo de un espacio de innovación'},
        {id: 'tech-summit', category: 'Tecnología', title: 'Global Tech Summit: Cloud & AI', description: 'El congreso tecnológico que reúne a directores de tecnología, fundadores de startups e ingenieros de software pioneros en Latinoamérica.', place: 'Ciudad Digital & Streaming', imageAlt: 'Auditorio de conferencia tecnológica con pantallas y público'},
        {id: 'biostartups', category: 'Agroinnovación', title: 'Feria de Biostartups Amazónicas', description: 'Emprendimientos de biotecnología y agroinnovación de la Amazonía peruana muestran sus productos a compradores y aliados.', place: 'Campus UNAS, Tingo María', imageAlt: 'Asistentes recorriendo stands con prototipos en una feria'},
      ],
    },
    offer: {
      title: 'Lo que Ofrecemos para Asistentes y Creadores',
      description: 'Una suite holística creada tanto para entusiastas que buscan conectar como para marcas que producen experiencias masivas.',
      items: [
        {icon: 'mentoring', title: 'Mentoría', description: 'Acompañamiento personalizado y soporte en fondos concursables.'},
        {icon: 'training', title: 'Capacitaciones', description: 'Formación para fortalecer ideas y negocios sostenibles.'},
        {icon: 'networking', title: 'Networking', description: 'Acceso a una red de emprendedores y mentores.'},
        {icon: 'coworking', title: 'Coworking', description: 'Espacios de trabajo colaborativo totalmente equipados.'},
      ],
    },
    footer: {rights: 'Todos los derechos reservados.'},
  },
  en: {
    skipLink: 'Skip to content',
    nav: {label: 'Main navigation', logoLabel: 'Nexo, back to home', access: 'Sign in'},
    hero: {
      titleLead: 'Connect, Subscribe and Create',
      titleHighlight: 'Extraordinary Events',
      description: 'We are the business incubator of the Universidad Nacional Agraria de la Selva. We provide advice and technical assistance to build innovative ventures.',
      explore: 'Explore events',
      create: 'Create my event',
    },
    events: {
      label: 'Featured events',
      register: 'Register',
      details: 'View details',
      createYours: 'Create yours',
      previous: 'Show previous events',
      next: 'Show more events',
      status: {pending: 'Upcoming', inProgress: 'In progress', finished: 'Finished'},
      items: [
        {id: 'demo-day', category: 'Entrepreneurship', title: 'IncubaUNAS Demo Day', description: 'Incubated startups pitch their projects to mentors, investors and ecosystem partners.', place: 'UNAS Auditorium', imageAlt: 'Auditorium with an audience facing a stage with screens'},
        {id: 'masterclass', category: 'Masterclass', title: 'Innovation and Venture Capital Masterclass', description: 'Learn institutional fundraising strategies, seed round structuring and accelerated scaling with investment partners.', place: 'Main Campus & Online', imageAlt: 'Collaborative workshop with people working at tables with laptops'},
        {id: 'product-forum', category: 'Design', title: 'Product Experience Forum & Expo', description: 'An exclusive gathering for experience designers, product leaders and creative directors focused on next-generation interfaces.', place: 'Innovation Hub', imageAlt: 'Product design exhibition with interactive prototypes'},
        {id: 'encuentro-innovacion', category: 'Innovation', title: 'Innovation Meetup', description: 'A day to connect entrepreneurs, researchers and companies around projects with impact in the region.', place: 'Innovation Pavilion', imageAlt: 'People talking at work tables in an innovation space'},
        {id: 'tech-summit', category: 'Technology', title: 'Global Tech Summit: Cloud & AI', description: 'The tech conference bringing together CTOs, startup founders and pioneering software engineers across Latin America.', place: 'Digital City & Streaming', imageAlt: 'Tech conference auditorium with screens and audience'},
        {id: 'biostartups', category: 'Agro-innovation', title: 'Amazonian Biostartups Fair', description: 'Biotech and agro-innovation ventures from the Peruvian Amazon showcase their products to buyers and partners.', place: 'UNAS Campus, Tingo María', imageAlt: 'Attendees walking past booths with prototypes at a fair'},
      ],
    },
    offer: {
      title: 'What We Offer to Attendees and Creators',
      description: 'A holistic suite built both for enthusiasts looking to connect and for brands producing large-scale experiences.',
      items: [
        {icon: 'mentoring', title: 'Mentoring', description: 'Personalized guidance and support for competitive funding.'},
        {icon: 'training', title: 'Training', description: 'Courses to strengthen ideas and sustainable businesses.'},
        {icon: 'networking', title: 'Networking', description: 'Access to a network of entrepreneurs and mentors.'},
        {icon: 'coworking', title: 'Coworking', description: 'Fully equipped collaborative workspaces.'},
      ],
    },
    footer: {rights: 'All rights reserved.'},
  },
};
