import type {HomeContent} from './content';

type HomeFooterProps = {content: HomeContent['footer']; year?: number};

export function HomeFooter({content, year = new Date().getFullYear()}: HomeFooterProps) {
  return (
    <footer className="nx-home-footer">
      <div className="nx-home-container">
        <p>© {year} <strong>NEXO</strong>. {content.rights}</p>
      </div>
    </footer>
  );
}
