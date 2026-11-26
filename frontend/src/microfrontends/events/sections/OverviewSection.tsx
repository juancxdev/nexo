import {
  ArrowRight,
  CalendarDays,
  CheckCircle2,
  MapPin,
  Users,
} from "lucide-react";
import { translateEvents as tr } from "../../../core/events-i18n";
import { displayDate, type RecordData } from "../api";
import { Detail, Empty, EventList, Panel, Stat } from "../components";
import type { Section } from "./shared";

interface OverviewSectionProps {
  events: RecordData[];
  venuesCount: number;
  speakersCount: number;
  isReader: boolean;
  navigate: (section: Section) => void;
  onSelectEvent: (id: string) => void;
  onCreateEvent: () => void;
}

export function OverviewSection({
  events,
  venuesCount,
  speakersCount,
  isReader,
  navigate,
  onSelectEvent,
  onCreateEvent,
}: OverviewSectionProps) {
  const publishedCount = events.filter((e) => e.status === "published").length;
  const draftCount = events.filter((e) => e.status === "draft").length;
  const otherCount = events.length - publishedCount - draftCount;

  const upcomingEvent = [...events]
    .filter((e) => e.starts_at && new Date(e.starts_at).getTime() >= Date.now())
    .sort(
      (a, b) =>
        new Date(a.starts_at || 0).getTime() -
        new Date(b.starts_at || 0).getTime(),
    )[0];

  return (
    <>
      <div className="stats-grid">
        <Stat
          icon={<CalendarDays size={18} />}
          label={tr("Eventos")}
          value={events.length}
          hint={tr("Registrados en total")}
          tone="peach"
        />
        <Stat
          icon={<CheckCircle2 size={18} />}
          label={tr("Publicados")}
          value={publishedCount}
          hint={tr("Activos actualmente")}
          tone="green"
        />
        <Stat
          icon={<MapPin size={18} />}
          label={tr("Sedes y salas")}
          value={venuesCount}
          hint={tr("Espacios disponibles")}
          tone="lavender"
        />
        <Stat
          icon={<Users size={18} />}
          label={tr("Ponentes")}
          value={speakersCount}
          hint={tr("Perfiles registrados")}
          tone="yellow"
        />
      </div>

      <div className="overview-grid">
        <Panel
          title={tr("Eventos recientes")}
          subtitle={tr("Haz clic en un evento para ver su detalle.")}
          action={
            <button className="link-button" onClick={() => navigate("events")}>
              {tr("Ver todos")} <ArrowRight size={15} />
            </button>
          }
        >
          {events.length ? (
            <EventList
              rows={events.slice(0, 5)}
              select={(event) => {
                onSelectEvent(event.id ?? "");
                navigate("events");
              }}
            />
          ) : (
            <Empty
              title={tr("Todavía no hay eventos")}
              description={tr(
                "Cuando existan eventos en la organización aparecerán aquí.",
              )}
              action={
                !isReader && (
                  <button className="text-action" onClick={onCreateEvent}>
                    {tr("Crear mi primer evento")} <ArrowRight size={14} />
                  </button>
                )
              }
            />
          )}
        </Panel>

        <Panel
          title={tr("Vista rápida")}
          subtitle={tr("Estado general y accesos directos.")}
        >
          <div className="stack">
            <Detail
              label={tr("Próximo evento")}
              value={
                upcomingEvent
                  ? `${upcomingEvent.title} · ${displayDate(upcomingEvent.starts_at)}`
                  : tr("Sin eventos próximos")
              }
            />
            <Detail
              label={tr("Distribución por estado")}
              value={`${publishedCount} ${tr("publicados")} · ${draftCount} ${tr("borradores")} · ${otherCount} ${tr("cerrados/otros")}`}
            />
            <div className="button-row">
              <button
                className="button secondary compact"
                onClick={() => navigate("agenda")}
              >
                {tr("Ir a agenda")} <ArrowRight size={14} />
              </button>
              <button
                className="button secondary compact"
                onClick={() => navigate("forms")}
              >
                {tr("Ir a formularios")} <ArrowRight size={14} />
              </button>
            </div>
          </div>
        </Panel>
      </div>
    </>
  );
}
