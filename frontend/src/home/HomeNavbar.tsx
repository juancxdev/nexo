import type {HomeContent} from './content';

type HomeNavbarProps = {content: HomeContent['nav']; onAccess: () => void};

export function HomeNavbar({content, onAccess}: HomeNavbarProps) {
  return (
    <header className="nx-home-nav">
      <nav className="nx-home-container nx-home-nav-inner" aria-label={content.label}>
        <a className="nx-home-logo" href="#inicio" aria-label={content.logoLabel}>
          <span className="nx-brand-mark" aria-hidden="true">N</span>
          <span className="nx-home-logo-name">Nexo</span>
        </a>
        <button type="button" className="nx-home-button nx-home-button-primary" onClick={onAccess}>
          {content.access}
        </button>
      </nav>
    </header>
  );
}
