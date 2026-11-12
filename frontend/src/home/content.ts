import type {Locale} from '../core/i18n';
import {homeImages} from './images';

export type EventStatus = 'live' | 'open' | 'lastSeats';
export type HomeEvent = {
  id: string;
  status: EventStatus;
  statusLabel: string;
  category: string;
  date: string;
  title: string;
  description: string;
  place: string;
  image: string;
  imageAlt: string;
};
export type MetricTone = 'default' | 'primary' | 'accent';
export type HomeMetric = {value: string; label: string; tone: MetricTone};
export type AwardIcon = 'trophy' | 'award' | 'medal';
export type OfferIcon = 'mentoring' | 'training' | 'networking' | 'coworking';

export type HomeContent = {
  skipLink: string;
  nav: {label: string; logoLabel: string; access: string};
  hero: {titleLead: string; titleHighlight: string; description: string; explore: string; create: string};
  events: {label: string; register: string; items: HomeEvent[]};
  metrics: {label: string; items: HomeMetric[]};
  awards: {title: string; description: string; items: {icon: AwardIcon; title: string}[]};
  offer: {title: string; description: string; items: {icon: OfferIcon; title: string; description: string}[]};
  allies: {title: string; description: string; imageAlt: string};
  footer: {rights: string};
};

const images = {
  techSummit: homeImages.techSummit,
  masterclass: homeImages.masterclass,
  productForum: homeImages.productForum,
};

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
      items: [
        {id: 'tech-summit', status: 'live', statusLabel: 'En vivo', category: 'Tecnología', date: '14 - 16 Noviembre, 2025', title: 'Global Tech Summit 2025: Cloud & AI', description: 'El congreso tecnológico líder que reúne a directores de tecnología, fundadores de startups e ingenieros de software pioneros en Latinoamérica.', place: 'Ciudad Digital & Streaming', image: images.techSummit, imageAlt: 'Auditorio de conferencia tecnológica con pantallas y público'},
        {id: 'masterclass', status: 'open', statusLabel: 'Inscripciones abiertas', category: 'Masterclass', date: '28 Noviembre, 2025', title: 'Masterclass de Innovación y Venture Capital', description: 'Aprende estrategias de levantamiento de capital institucional, estructuración de rondas seed y escalamiento acelerado con socios inversores.', place: 'Campus Central & Online', image: images.masterclass, imageAlt: 'Taller colaborativo con personas trabajando en mesas con laptops'},
        {id: 'product-forum', status: 'lastSeats', statusLabel: 'Últimos cupos', category: 'Diseño', date: '05 Diciembre, 2025', title: 'Product Experience Forum & Expo', description: 'Un encuentro exclusivo para diseñadores de experiencia, líderes de producto y directores creativos enfocados en interfaces de próxima generación.', place: 'Hub de Innovación', image: images.productForum, imageAlt: 'Exposición de diseño de producto con prototipos interactivos'},
      ],
    },
    metrics: {
      label: 'Cifras clave',
      items: [
        {value: '+10', label: 'Programas', tone: 'default'},
        {value: '+300', label: 'Emprendedores impactados', tone: 'default'},
        {value: '+40', label: 'Startups incubadas', tone: 'accent'},
        {value: '$500k', label: 'Capital semilla', tone: 'default'},
        {value: '+20', label: 'Mentores', tone: 'primary'},
        {value: '+8', label: 'Startups financiadas', tone: 'default'},
        {value: '+30', label: 'Talleres', tone: 'default'},
        {value: '+2', label: 'Congresos', tone: 'accent'},
      ],
    },
    awards: {
      title: 'Premios y Reconocimientos',
      description: 'Avalados por organismos internacionales de tecnología, auditoría de pagos y gremios de organizadores profesionales.',
      items: [
        {icon: 'trophy', title: 'Primera incubadora en llevar emprendimientos amazónicos a Europa'},
        {icon: 'award', title: '2 startups en Programas de Mentor Day - España'},
        {icon: 'medal', title: 'Articulación con Gobiernos regionales y toda la Red CITE'},
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
    allies: {
      title: 'Nuestros Aliados',
      description: 'Empresas e instituciones que confían y colaboran con nuestra incubadora',
      imageAlt: 'Logotipos de las instituciones y empresas aliadas',
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
      items: [
        {id: 'tech-summit', status: 'live', statusLabel: 'Live', category: 'Technology', date: 'November 14 - 16, 2025', title: 'Global Tech Summit 2025: Cloud & AI', description: 'The leading tech conference bringing together CTOs, startup founders and pioneering software engineers across Latin America.', place: 'Digital City & Streaming', image: images.techSummit, imageAlt: 'Tech conference auditorium with screens and audience'},
        {id: 'masterclass', status: 'open', statusLabel: 'Registration open', category: 'Masterclass', date: 'November 28, 2025', title: 'Innovation and Venture Capital Masterclass', description: 'Learn institutional fundraising strategies, seed round structuring and accelerated scaling with investment partners.', place: 'Main Campus & Online', image: images.masterclass, imageAlt: 'Collaborative workshop with people working at tables with laptops'},
        {id: 'product-forum', status: 'lastSeats', statusLabel: 'Last seats', category: 'Design', date: 'December 05, 2025', title: 'Product Experience Forum & Expo', description: 'An exclusive gathering for experience designers, product leaders and creative directors focused on next-generation interfaces.', place: 'Innovation Hub', image: images.productForum, imageAlt: 'Product design exhibition with interactive prototypes'},
      ],
    },
    metrics: {
      label: 'Key figures',
      items: [
        {value: '+10', label: 'Programs', tone: 'default'},
        {value: '+300', label: 'Entrepreneurs reached', tone: 'default'},
        {value: '+40', label: 'Incubated startups', tone: 'accent'},
        {value: '$500k', label: 'Seed capital', tone: 'default'},
        {value: '+20', label: 'Mentors', tone: 'primary'},
        {value: '+8', label: 'Funded startups', tone: 'default'},
        {value: '+30', label: 'Workshops', tone: 'default'},
        {value: '+2', label: 'Conferences', tone: 'accent'},
      ],
    },
    awards: {
      title: 'Awards and Recognition',
      description: 'Endorsed by international technology bodies, payment auditors and professional event organizer associations.',
      items: [
        {icon: 'trophy', title: 'First incubator to take Amazonian ventures to Europe'},
        {icon: 'award', title: '2 startups in Mentor Day programs - Spain'},
        {icon: 'medal', title: 'Partnership with regional governments and the entire CITE Network'},
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
    allies: {
      title: 'Our Partners',
      description: 'Companies and institutions that trust and collaborate with our incubator',
      imageAlt: 'Logos of partner institutions and companies',
    },
    footer: {rights: 'All rights reserved.'},
  },
};
