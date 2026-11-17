import { useCallback, useEffect, useState } from "react";
import { Activity, Layers3, Plus, Users } from "lucide-react";
import { translateEvents as tr } from "../../../core/events-i18n";
import { displayDate, items, type EventsRequest, type RecordData } from "../api";
import { Badge, Detail, Empty, Panel } from "../components";
import {
  textField,
  type InspectFn,
  type OpenDialogFn,
  type RunFn,
  type ShowErrorFn,
} from "./shared";

interface ActivitySectionProps {
  selected?: RecordData;
  selectedPath: string;
  isReader: boolean;
  request: EventsRequest;
  inspect: InspectFn;
  setDialog: OpenDialogFn;
  run: RunFn;
  showError: ShowErrorFn;
}

const ACTION_LABELS: Record<string, string> = {
  CREATE: "Creación de evento",
  UPDATE: "Actualización de evento",
  PUBLISH: "Publicación de versión",
  CLOSE: "Cierre de inscripciones",
  CANCEL: "Cancelación de evento",
  ARCHIVE: "Archivado de evento",
  DUPLICATE: "Duplicación de evento",
  ASSIGN: "Asignación de responsable",
  REVOKE_ASSIGNMENT: "Revocación de asignación",
  created: "Recurso creado",
  created_draft: "Sesión creada en borrador",
  scheduled: "Sesión programada",
  updated: "Recurso actualizado",
  cancelled: "Sesión cancelada",
  completed: "Sesión completada",
  archived: "Recurso archivado",
};

function formatDiffSummary(diff: unknown): string {
  if (!diff || typeof diff !== "object") return "";
  const record = diff as Record<string, unknown>;
  const parts: string[] = [];
  if (Array.isArray(record.changed_fields) && record.changed_fields.length) {
    parts.push(`${tr("Campos")}: ${record.changed_fields.join(", ")}`);
  }
  if (Array.isArray(record.fields) && record.fields.length) {
    parts.push(`${tr("Campos")}: ${record.fields.join(", ")}`);
  }
  if (typeof record.reason === "string" && record.reason) {
    parts.push(`${tr("Motivo")}: ${record.reason}`);
  }
  if (typeof record.status_reason === "string" && record.status_reason) {
    parts.push(`${tr("Motivo")}: ${record.status_reason}`);
  }
  if (typeof record.role === "string" && record.role) {
    parts.push(`${tr("Rol")}: ${record.role}`);
  }
  if (typeof record.release_version === "number") {
    parts.push(`Release v${record.release_version}`);
  }
  return parts.join(" · ");
}

export function ActivitySection({
  selected,
  selectedPath,
  isReader,
  request,
  inspect,
  setDialog,
  run,
  showError,
}: ActivitySectionProps) {
  const [view, setView] = useState<"history" | "audit" | "revisions">("history");
  const [revisions, setRevisions] = useState<RecordData[]>([]);
  const [auditTrail, setAuditTrail] = useState<RecordData[]>([]);
  const [assignments, setAssignments] = useState<RecordData[]>([]);

  const loadActivity = useCallback(async () => {
    if (!selected?.id) {
      setRevisions([]);
      setAuditTrail([]);
      setAssignments([]);
      return;
    }
    try {
      const [historyData, assignmentsData] = await Promise.all([
        request(`${selectedPath}/history`),
        request(`${selectedPath}/assignments`),
      ]);
      setRevisions(
        Array.isArray(historyData?.revisions)
          ? (historyData.revisions as RecordData[])
          : items(historyData),
      );
      setAuditTrail(
        Array.isArray(historyData?.audit_trail)
          ? (historyData.audit_trail as RecordData[])
          : [],
      );
      setAssignments(items(assignmentsData));
    } catch (error) {
      showError(error);
    }
  }, [selected?.id, selectedPath, request, showError]);

  useEffect(() => {
    void loadActivity();
  }, [loadActivity, selected?.revision_number]);

  if (!selected) {
    return (
      <Empty
        title={tr("Selecciona un evento")}
        description={tr("Elige un evento para consultar sus registros.")}
      />
    );
  }

  const openAssignDialog = () =>
    setDialog({
      title: tr("Asignar responsable"),
      fields: [
        textField("user_id", tr("ID de usuario (UUID)"), true),
        textField("role", tr("Rol"), true),
        textField("scopes", tr("Permisos separados por comas"), true),
      ],
      initial: {
        role: "event_manager",
        scopes: "events:read,events:write",
      },
      submit: async (body: RecordData) => {
        await run(`${selectedPath}/assignments`, "POST", {
          ...body,
          scopes: String(body.scopes)
            .split(",")
            .map((s) => s.trim())
            .filter(Boolean),
        });
        await loadActivity();
      },
    });

  const revokeAssignment = async (row: RecordData) => {
    try {
      await run(
        `${selectedPath}/assignments/${row.id}`,
        "DELETE",
        undefined,
        undefined,
        tr("Asignación revocada."),
      );
      await loadActivity();
    } catch (error) {
      showError(error);
    }
  };

  return (
    <div className="stack">
      <Panel
        title={tr("Historial y auditoría")}
        subtitle={tr(
          "Registro inmutable de revisiones y operaciones del evento (EC-041).",
        )}
        action={
          <div className="button-row">
            {(
              [
                ["history", tr("Historial")],
                ["audit", tr("Auditoría")],
                ["revisions", tr("Revisiones")],
              ] as const
            ).map(([id, label]) => (
              <button
                key={id}
                className={`button ${view === id ? "primary" : "secondary"} compact`}
                onClick={() => setView(id)}
              >
                {label}
              </button>
            ))}
            <button
              className="button secondary compact"
              onClick={() => void inspect(tr("Historial"), `${selectedPath}/${view}`)}
            >
              {tr("Ver JSON")}
            </button>
          </div>
        }
      >
        <div className="detail-grid">
          <Detail
            label={tr("Revisión actual")}
            value={`REV. ${selected.revision_number ?? 1}`}
          />
          <Detail
            label={tr("Revisiones")}
            value={String(revisions.length)}
          />
          <Detail
            label={tr("Auditoría")}
            value={String(auditTrail.length)}
          />
          <Detail
            label={tr("Asignaciones")}
            value={String(
              assignments.filter((a) => a.status === "active").length,
            )}
          />
        </div>

        {(view === "history" || view === "audit") && (
          <div className="stack">
            <strong>{tr("Auditoría")}</strong>
            {auditTrail.length ? (
              <div className="resource-list">
                {auditTrail.map((row) => {
                  const actionKey = String(row.action || "");
                  const actionLabel = tr(
                    ACTION_LABELS[actionKey] || actionKey || "Operación",
                  );
                  const roles = Array.isArray(row.actor_roles)
                    ? row.actor_roles.join(", ")
                    : "";
                  const diffSummary = formatDiffSummary(row.diff);
                  const hashPreview = row.payload_hash
                    ? `${String(row.payload_hash).slice(0, 12)}…`
                    : "";
                  return (
                    <div className="resource-row" key={String(row.id)}>
                      <div className="resource-icon green">
                        <Activity size={18} />
                      </div>
                      <div className="resource-main">
                        <strong>
                          {actionLabel}{" "}
                          {row.entity_type ? `(${String(row.entity_type)})` : ""}
                        </strong>
                        <span>
                          {tr("Actor")}: {String(row.actor_id || "—")}
                          {roles ? ` [${roles}]` : ""} ·{" "}
                          {displayDate(String(row.created_at || ""))}
                          {hashPreview ? ` · Hash: ${hashPreview}` : ""}
                          {diffSummary ? ` · ${diffSummary}` : ""}
                        </span>
                      </div>
                      {row.revision_number !== undefined &&
                        row.revision_number !== null && (
                          <span className="revision">
                            REV. {String(row.revision_number)}
                          </span>
                        )}
                    </div>
                  );
                })}
              </div>
            ) : (
              <Empty
                title={tr("Sin registros por ahora")}
                description={tr("Cuando agregues elementos aparecerán aquí.")}
              />
            )}
          </div>
        )}

        {(view === "history" || view === "revisions") && (
          <div className="stack">
            <strong>{tr("Revisiones")}</strong>
            {revisions.length ? (
              <div className="resource-list">
                {revisions.map((row) => {
                  const snap = (row.snapshot || {}) as RecordData;
                  return (
                    <div
                      className="resource-row"
                      key={String(row.id || row.revision_number)}
                    >
                      <div className="resource-icon lavender">
                        <Layers3 size={18} />
                      </div>
                      <div className="resource-main">
                        <strong>
                          REV. {String(row.revision_number ?? "")} —{" "}
                          {String(snap.title || selected.title || "")}
                        </strong>
                        <span>
                          {String(
                            row.change_reason || tr("Instantánea inmutable"),
                          )}{" "}
                          · {tr("Autor")}: {String(row.changed_by || "—")} ·{" "}
                          {displayDate(String(row.created_at || ""))}
                        </span>
                      </div>
                      <Badge
                        status={String(snap.status || selected.status || "")}
                      />
                    </div>
                  );
                })}
              </div>
            ) : (
              <Empty
                title={tr("Sin registros por ahora")}
                description={tr("Cuando agregues elementos aparecerán aquí.")}
              />
            )}
          </div>
        )}
      </Panel>

      <Panel
        title={tr("Trazabilidad y responsables")}
        subtitle={tr("Consulta los registros y asignaciones del evento.")}
        action={
          <div className="button-row">
            <span className="count-pill">
              {assignments.length} {tr("Asignaciones")}
            </span>
            {!isReader && (
              <button
                className="button primary compact"
                onClick={openAssignDialog}
              >
                <Plus size={15} /> {tr("Asignar responsable")}
              </button>
            )}
          </div>
        }
      >
        {assignments.length ? (
          <div className="resource-list">
            {assignments.map((row) => (
              <div className="resource-row" key={String(row.id)}>
                <div className="resource-icon peach">
                  <Users size={18} />
                </div>
                <div className="resource-main">
                  <strong>
                    {String(row.role || "")} · {String(row.user_id || "")}
                  </strong>
                  <span>
                    {tr("Permisos")}:{" "}
                    {Array.isArray(row.scopes)
                      ? row.scopes.join(", ")
                      : String(row.scopes || "—")}{" "}
                    · {displayDate(String(row.created_at || ""))}
                  </span>
                </div>
                <Badge status={String(row.status || "")} />
                {!isReader && row.status === "active" && (
                  <button
                    className="small-button muted"
                    onClick={() => void revokeAssignment(row)}
                  >
                    {tr("Revocar")}
                  </button>
                )}
              </div>
            ))}
          </div>
        ) : (
          <Empty
            title={tr("Sin registros por ahora")}
            description={tr("Cuando agregues elementos aparecerán aquí.")}
          />
        )}
      </Panel>
    </div>
  );
}
