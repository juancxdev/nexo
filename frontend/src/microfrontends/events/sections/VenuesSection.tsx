import { Plus } from "lucide-react";
import { translateEvents as tr } from "../../../core/events-i18n";
import type { RecordData } from "../api";
import { Panel, ResourceList } from "../components";
import {
  cleanPayload,
  selectOption,
  textField,
  type OpenDialogFn,
  type RunFn,
} from "./shared";

interface VenuesSectionProps {
  venues: RecordData[];
  isReader: boolean;
  onCreateVenue: () => void;
  onCreateRoom: () => void;
  setDialog: OpenDialogFn;
  run: RunFn;
  archiveResource: (path: string, row: RecordData) => Promise<void>;
}

export function VenuesSection({
  venues,
  isReader,
  onCreateVenue,
  onCreateRoom,
  setDialog,
  run,
  archiveResource,
}: VenuesSectionProps) {
  const parentVenues = venues.filter((v) => v.kind === "venue");
  const rooms = venues
    .filter((v) => v.kind === "room")
    .map((r) => ({
      ...r,
      parent_venue_name: parentVenues.find((v) => v.id === r.parent_venue_id)
        ?.name,
    }));

  const editVenue = (row: RecordData) =>
    setDialog({
      title: tr("Editar sede"),
      fields: [
        textField("name", tr("Nombre de la sede"), true),
        textField("address", tr("Dirección"), true),
        textField("locality", tr("Ciudad")),
        {
          name: "capacity",
          label: tr("Capacidad"),
          type: "number",
        },
        {
          name: "exclusive",
          label: tr("Recurso de uso exclusivo"),
          type: "checkbox",
        },
      ],
      initial: {
        ...row,
        address: row.location?.address ?? "",
        locality: row.location?.locality ?? "",
      },
      submit: async (body: RecordData) => {
        const { address, locality, ...rest } = body;
        await run(
          `/venues/${row.id}`,
          "PATCH",
          {
            ...cleanPayload(rest),
            ...(address
              ? { location: cleanPayload({ address, locality }) }
              : {}),
          },
          row.revision,
        );
      },
    });

  const editRoom = (row: RecordData) =>
    setDialog({
      title: tr("Editar sala"),
      fields: [
        {
          name: "parent_venue_id",
          label: tr("Sede principal"),
          type: "select",
          required: true,
          options: parentVenues.map((v) => selectOption(v, v.name)),
        },
        textField("name", tr("Nombre de la sala"), true),
        textField("address", tr("Ubicación o piso (opcional)")),
        {
          name: "capacity",
          label: tr("Capacidad"),
          type: "number",
        },
        {
          name: "exclusive",
          label: tr("Recurso de uso exclusivo"),
          type: "checkbox",
        },
      ],
      initial: {
        ...row,
        address: row.location?.address ?? "",
      },
      submit: async (body: RecordData) => {
        const { address, ...rest } = body;
        const parent = parentVenues.find((v) => v.id === body.parent_venue_id);
        const resolvedAddress =
          String(address || "").trim() ||
          parent?.location?.address ||
          row.location?.address ||
          "Sede principal";
        await run(
          `/venues/${row.id}`,
          "PATCH",
          {
            ...cleanPayload(rest),
            location: cleanPayload({
              address: resolvedAddress,
              locality: parent?.location?.locality ?? row.location?.locality,
            }),
          },
          row.revision,
        );
      },
    });

  return (
    <div className="stack">
      <Panel
        title={tr("Sedes")}
        subtitle={tr("Recintos, campus o edificios principales de tu organización.")}
        action={
          <div className="button-row">
            <span className="count-pill">
              {parentVenues.length} {tr("sedes")}
            </span>
            {!isReader && (
              <button
                className="button primary compact"
                onClick={onCreateVenue}
              >
                <Plus size={15} /> {tr("Nueva sede")}
              </button>
            )}
          </div>
        }
      >
        <ResourceList
          rows={parentVenues}
          kind="venue"
          onEdit={editVenue}
          onArchive={(row) => void archiveResource(`/venues/${row.id}`, row)}
        />
      </Panel>

      <Panel
        title={tr("Salas")}
        subtitle={tr("Auditorios, aulas o ambientes específicos dentro de una sede.")}
        action={
          <div className="button-row">
            <span className="count-pill">
              {rooms.length} {tr("salas")}
            </span>
            {!isReader && (
              <button
                className="button primary compact"
                onClick={onCreateRoom}
              >
                <Plus size={15} /> {tr("Nueva sala")}
              </button>
            )}
          </div>
        }
      >
        <ResourceList
          rows={rooms}
          kind="venue"
          onEdit={editRoom}
          onArchive={(row) => void archiveResource(`/venues/${row.id}`, row)}
        />
      </Panel>
    </div>
  );
}
