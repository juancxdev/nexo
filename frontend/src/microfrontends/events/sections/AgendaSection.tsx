import { Clock3, Plus } from "lucide-react";
import { translateEvents as tr } from "../../../core/events-i18n";
import { displayDate, type RecordData } from "../api";
import { Badge, Empty, Panel, ResourceList } from "../components";
import {
  cleanPayload,
  dateField,
  selectOption,
  textField,
  type CommandFn,
  type OpenDialogFn,
  type RunFn,
} from "./shared";

interface AgendaSectionProps {
  selected?: RecordData;
  selectedPath: string;
  programs: RecordData[];
  sessions: RecordData[];
  venues: RecordData[];
  canWrite: boolean;
  isReader: boolean;
  onCreateProgram: () => void;
  onCreateSession: () => void;
  onScheduleSession: (session: RecordData) => Promise<void>;
  setDialog: OpenDialogFn;
  run: RunFn;
  command: CommandFn;
  archiveResource: (path: string, row: RecordData) => Promise<void>;
}

export function AgendaSection({
  selected,
  selectedPath,
  programs,
  sessions,
  venues,
  canWrite,
  isReader,
  onCreateProgram,
  onCreateSession,
  onScheduleSession,
  setDialog,
  run,
  command,
  archiveResource,
}: AgendaSectionProps) {
  if (!selected) {
    return (
      <Empty
        title={tr("Selecciona un evento")}
        description={tr("Elige un evento arriba para administrar su agenda.")}
      />
    );
  }

  const rooms = venues.filter((v) => v.kind === "room");
  const roomOptions = rooms.map((r) => {
    const parent = venues.find((v) => v.id === r.parent_venue_id);
    return selectOption(r, parent ? `${r.name} (${parent.name})` : r.name);
  });

  const formatVenueLabel = (venueId?: unknown) => {
    const venue = venues.find((v) => v.id === venueId);
    if (!venue) return tr("Sin sala");
    if (venue.kind === "room" && venue.parent_venue_id) {
      const parent = venues.find((v) => v.id === venue.parent_venue_id);
      return parent ? `${venue.name} (${parent.name})` : String(venue.name);
    }
    return String(venue.name);
  };

  const editSession = (row: RecordData) =>
    setDialog({
      title: tr("Editar sesión"),
      fields: [
        textField("title", tr("Título"), true),
        {
          name: "program_id",
          label: tr("Programa"),
          type: "select",
          required: true,
          options: programs.map((p) => selectOption(p, p.name)),
        },
        {
          name: "venue_id",
          label: tr("Sala"),
          type: "select",
          required: true,
          options: roomOptions,
        },
        dateField("starts_at", tr("Inicio")),
        dateField("ends_at", tr("Fin")),
        { name: "description", label: tr("Descripción"), type: "textarea" },
      ],
      initial: row,
      submit: async (body: RecordData) => {
        await run(
          `${selectedPath}/sessions/${row.id}`,
          "PATCH",
          cleanPayload(body),
          row.revision,
          tr("Sesión actualizada."),
        );
      },
    });

  const cancelSession = (row: RecordData) =>
    setDialog({
      title: tr("Cancelar sesión"),
      description: tr("Libera la sala y ponentes reservados para este horario."),
      fields: [
        {
          name: "reason",
          label: tr("Motivo de cancelación"),
          type: "textarea",
          required: true,
        },
      ],
      submit: async (body: RecordData) => {
        await run(
          `${selectedPath}/sessions/${row.id}/cancellations`,
          "POST",
          cleanPayload(body),
          row.revision,
          tr("Sesión cancelada."),
        );
      },
    });

  const completeSession = async (row: RecordData) => {
    await run(
      `${selectedPath}/sessions/${row.id}/completions`,
      "POST",
      {},
      row.revision,
      tr("Sesión marcada como completada."),
    );
  };

  return (
    <div className="stack">
      <Panel
        title={tr("Programas")}
        subtitle={tr("Agrupa las sesiones por tema o itinerario.")}
        action={
          !isReader && (
            <button
              className="button primary compact"
              onClick={onCreateProgram}
            >
              <Plus size={15} /> {tr("Nuevo programa")}
            </button>
          )
        }
      >
        <ResourceList
          rows={programs}
          kind="program"
          onEdit={(row) =>
            setDialog({
              title: tr("Editar programa"),
              fields: [
                textField("name", tr("Nombre")),
                textField("slug", tr("Identificador")),
                {
                  name: "display_order",
                  label: tr("Orden"),
                  type: "number",
                },
                textField("color", tr("Color (#RRGGBB)")),
                {
                  name: "description",
                  label: tr("Descripción"),
                  type: "textarea",
                },
              ],
              initial: row,
              submit: async (body) => {
                await run(
                  `${selectedPath}/programs/${row.id}`,
                  "PATCH",
                  cleanPayload(body),
                  row.revision,
                );
              },
            })
          }
          onArchive={(row) =>
            void archiveResource(`${selectedPath}/programs/${row.id}`, row)
          }
        />
      </Panel>

      <Panel
        title={tr("Sesiones")}
        subtitle={tr("Crea borradores y reserva los recursos al programarlos.")}
        action={
          <div className="button-row">
            {canWrite && (
              <button
                className="button secondary compact"
                onClick={() =>
                  command(
                    tr("Validar agenda"),
                    `${selectedPath}/schedule-validations`,
                    [
                      {
                        name: "expected_event_revision",
                        label: tr("Revisión del evento"),
                        type: "number",
                        required: true,
                      },
                      {
                        name: "candidate_sessions",
                        label: tr("Sesiones candidatas (JSON)"),
                        type: "json",
                        required: true,
                        hint: tr(
                          "Para validar la agenda existente utiliza [].",
                        ),
                      },
                    ],
                    {
                      expected_event_revision: selected.revision_number,
                      candidate_sessions: [],
                    },
                  )
                }
              >
                {tr("Validar agenda")}
              </button>
            )}
            {!isReader && (
              <button
                className="button primary compact"
                onClick={onCreateSession}
              >
                <Plus size={15} /> {tr("Nueva sesión")}
              </button>
            )}
          </div>
        }
      >
        {sessions.length ? (
          <div className="resource-list">
            {sessions.map((row) => {
              const program = programs.find((p) => p.id === row.program_id);
              const programLabel = program ? program.name : tr("Sin programa");
              const venueLabel = formatVenueLabel(row.venue_id);
              const isMutable =
                row.status === "draft" || row.status === "scheduled";
              return (
                <div className="resource-row" key={row.id}>
                  <div className="resource-icon lavender">
                    <Clock3 size={18} />
                  </div>
                  <div className="resource-main">
                    <strong>{row.title}</strong>
                    <span>
                      {programLabel} · {venueLabel} · {displayDate(row.starts_at)}{" "}
                      · {displayDate(row.ends_at)}
                    </span>
                  </div>
                  <span className="revision">REV. {row.revision}</span>
                  <Badge status={row.status} />
                  {row.status === "draft" && !isReader && (
                    <button
                      className="small-button"
                      onClick={() => void onScheduleSession(row)}
                    >
                      {tr("Programar")}
                    </button>
                  )}
                  {isMutable && !isReader && (
                    <button
                      className="small-button"
                      onClick={() => editSession(row)}
                    >
                      {tr("Editar")}
                    </button>
                  )}
                  {row.status === "scheduled" && !isReader && (
                    <button
                      className="small-button"
                      onClick={() => void completeSession(row)}
                    >
                      {tr("Completar")}
                    </button>
                  )}
                  {isMutable && !isReader && (
                    <button
                      className="small-button muted"
                      onClick={() => cancelSession(row)}
                    >
                      {tr("Cancelar")}
                    </button>
                  )}
                </div>
              );
            })}
          </div>
        ) : (
          <Empty
            title={tr("Aún no hay sesiones")}
            description={tr(
              "Añade una sesión después de crear un programa y una sala.",
            )}
          />
        )}
      </Panel>
    </div>
  );
}
