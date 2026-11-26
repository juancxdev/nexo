import {
  useCallback,
  useEffect,
  useRef,
  useState,
  type FormEvent,
} from "react";
import { AlertCircle } from "lucide-react";
import { ApiError, type EventsTransport } from "../api";
import { Panel } from "../components";
import { translateEvents as tr } from "../../../core/events-i18n";

type Definition = {
  id: string;
  name: string;
  purpose: string;
};

type Version = {
  id: string;
  version_number: number;
  revision: number;
  status: string;
};

type Binding = {
  id: string;
  event_id: string;
  form_version_id?: string;
  purpose: string;
  status: string;
  opens_at?: string;
  closes_at?: string;
  revision: number;
};

type VersionDetail = {
  id: string;
  version_number: number;
  status: string;
  schema?: {
    properties?: Record<
      string,
      { type?: string; title?: string; minLength?: number }
    >;
    required?: string[];
  };
  ui_schema?: { order?: string[] };
  consent?: { text?: string; version?: string };
  field_policies?: Record<
    string,
    { purpose?: string; classification?: string; retention?: string }
  >;
};

function FormPreview({
  version,
  binding,
  selectedEventId,
}: {
  version: VersionDetail | null;
  binding?: Binding;
  selectedEventId?: string;
}) {
  const [values, setValues] = useState<Record<string, string | boolean>>({});
  const [accepted, setAccepted] = useState(false);
  const [previewMsg, setPreviewMsg] = useState("");
  const [previewErrors, setPreviewErrors] = useState<Record<string, string>>(
    {},
  );

  useEffect(() => {
    setValues({});
    setAccepted(false);
    setPreviewMsg("");
    setPreviewErrors({});
  }, [version?.id, selectedEventId]);

  if (!version) {
    return (
      <p>
        {selectedEventId
          ? tr(
              "Este evento no tiene un formulario disponible para previsualizar.",
            )
          : tr("Selecciona un evento o versión para ver la vista previa.")}
      </p>
    );
  }

  const props = version.schema?.properties || {};
  const required = new Set(version.schema?.required || []);
  const policies = version.field_policies || {};
  const orderedKeys = [
    ...(version.ui_schema?.order || []).filter((k) => k in props),
    ...Object.keys(props).filter(
      (k) => !(version.ui_schema?.order || []).includes(k),
    ),
  ];

  const handlePreviewSubmit = (e: FormEvent) => {
    e.preventDefault();
    setPreviewMsg("");
    const nextErrors: Record<string, string> = {};
    for (const key of orderedKeys) {
      if (!required.has(key)) continue;
      const def = props[key];
      const val = values[key];
      if (def.type === "boolean" ? !val : String(val ?? "").trim() === "") {
        nextErrors[key] = tr("Este campo es obligatorio.");
      }
    }
    if (version.consent?.text && !accepted) {
      nextErrors.__consent = tr(
        "Debes aceptar el consentimiento para continuar.",
      );
    }
    if (Object.keys(nextErrors).length > 0) {
      setPreviewErrors(nextErrors);
      return;
    }
    setPreviewErrors({});
    setPreviewMsg(
      tr(
        "Simulación validada localmente. La vista previa no guarda respuestas.",
      ),
    );
  };

  return (
    <div className="stack">
      <div className="info-banner">
        <div>
          <strong>
            {tr("Vista previa del formulario")} · v{version.version_number} (
            {version.status})
          </strong>
          <p>
            {binding
              ? `${tr("Propósito")}: ${binding.purpose} · ${tr("Vínculo")}: ${binding.status}`
              : tr(
                  "Simulación interactiva — no guarda respuestas en el servidor.",
                )}
          </p>
        </div>
      </div>

      {previewMsg && (
        <div className="notice" role="status">
          {previewMsg}
        </div>
      )}

      <form onSubmit={handlePreviewSubmit} noValidate className="form-grid">
        {orderedKeys.map((key) => {
          const def = props[key];
          const isRequired = required.has(key);
          const policy = policies[key];
          const errMsg = previewErrors[key];
          const errId = errMsg ? `preview-err-${key}` : undefined;
          if (def.type === "boolean") {
            return (
              <label
                className={`check-field ${errMsg ? "has-error" : ""}`}
                key={key}
              >
                <input
                  type="checkbox"
                  checked={Boolean(values[key])}
                  aria-invalid={Boolean(errMsg)}
                  aria-describedby={errId}
                  onChange={(e) => {
                    setPreviewErrors((prev) => ({ ...prev, [key]: "" }));
                    setValues({ ...values, [key]: e.target.checked });
                  }}
                />
                <span>
                  {def.title || key}
                  {isRequired ? " *" : ""}
                </span>
                {errMsg && (
                  <span id={errId} className="field-error" role="alert">
                    <AlertCircle size={14} /> {errMsg}
                  </span>
                )}
              </label>
            );
          }
          return (
            <label className={`field ${errMsg ? "has-error" : ""}`} key={key}>
              <span>
                {def.title || key}
                {isRequired ? " *" : ""}
              </span>
              <input
                type={
                  def.type === "number" || def.type === "integer"
                    ? "number"
                    : "text"
                }
                required={isRequired}
                value={String(values[key] ?? "")}
                placeholder={def.title || key}
                aria-invalid={Boolean(errMsg)}
                aria-describedby={errId}
                onChange={(e) => {
                  setPreviewErrors((prev) => ({ ...prev, [key]: "" }));
                  setValues({ ...values, [key]: e.target.value });
                }}
              />
              {policy?.classification && (
                <small>
                  {policy.classification}
                  {policy.retention ? ` · ${policy.retention}` : ""}
                </small>
              )}
              {errMsg && (
                <span id={errId} className="field-error" role="alert">
                  <AlertCircle size={14} /> {errMsg}
                </span>
              )}
            </label>
          );
        })}

        {version.consent?.text && (
          <label
            className={`check-field ${previewErrors.__consent ? "has-error" : ""}`}
          >
            <input
              type="checkbox"
              required
              checked={accepted}
              aria-invalid={Boolean(previewErrors.__consent)}
              onChange={(e) => {
                setPreviewErrors((prev) => ({ ...prev, __consent: "" }));
                setAccepted(e.target.checked);
              }}
            />
            <span>
              {version.consent.text} ({version.consent.version}) *
            </span>
            {previewErrors.__consent && (
              <span className="field-error" role="alert">
                <AlertCircle size={14} /> {previewErrors.__consent}
              </span>
            )}
          </label>
        )}

        <div className="form-actions">
          <button type="submit" className="button primary">
            {tr("Probar envío (vista previa)")}
          </button>
        </div>
      </form>
    </div>
  );
}

type FieldDraft = {
  name: string;
  title: string;
  type: string;
  purpose: string;
  classification: string;
  retention: string;
};

type Props = {
  transport: EventsTransport;
  selectedEventId?: string;
  canWrite: boolean;
  canPublish: boolean;
  contextKey: string;
};

export function FormsPanel({
  transport,
  selectedEventId,
  canWrite,
  canPublish,
  contextKey,
}: Props) {
  const [forms, setForms] = useState<Definition[]>([]);
  const [versions, setVersions] = useState<Version[]>([]);
  const [bindings, setBindings] = useState<Binding[]>([]);
  const [formId, setFormId] = useState("");
  const [versionId, setVersionId] = useState("");
  const [name, setName] = useState("");
  const [purpose, setPurpose] = useState("registration");
  const [fieldName, setFieldName] = useState("nombre");
  const [fieldTitle, setFieldTitle] = useState("Nombre");
  const [fieldType, setFieldType] = useState("string");
  const [extraFields, setExtraFields] = useState<FieldDraft[]>([]);
  const [consent, setConsent] = useState("");
  const [consentVersion, setConsentVersion] = useState("");
  const [policyPurpose, setPolicyPurpose] = useState("");
  const [classification, setClassification] = useState("interna");
  const [retention, setRetention] = useState("");
  const [bindingStatus, setBindingStatus] = useState("open");
  const [opens, setOpens] = useState("");
  const [closes, setCloses] = useState("");
  const [bindingId, setBindingId] = useState("");
  const [reason, setReason] = useState("");
  const [notice, setNotice] = useState("");
  const [busy, setBusy] = useState(false);
  const [formErrors, setFormErrors] = useState<Record<string, string>>({});
  const [activeTab, setActiveTab] = useState<"design" | "preview">("design");
  const [allFormVersions, setAllFormVersions] = useState<
    Record<string, Version[]>
  >({});
  const [previewVersionId, setPreviewVersionId] = useState("");
  const [previewVersion, setPreviewVersion] = useState<VersionDetail | null>(
    null,
  );
  const [draftEditor, setDraftEditor] = useState<{
    id: string;
    revision: number;
    schema: string;
    ui_schema: string;
    consent: string;
    field_policies: string;
  } | null>(null);

  const contextRef = useRef(contextKey);
  contextRef.current = contextKey;

  const selectionRef = useRef({ formId, selectedEventId });
  selectionRef.current = { formId, selectedEventId };

  const selected = versions.find((v) => v.id === versionId);

  const load = useCallback(async () => {
    const result = await transport.request<Definition[]>("/forms");
    if (contextRef.current !== contextKey) return;
    const formList = Array.isArray(result) ? result : [];
    setForms(formList);

    if (formId) {
      const rows = await transport.request<Version[]>(
        `/forms/${formId}/versions`,
      );
      if (
        contextRef.current !== contextKey ||
        selectionRef.current.formId !== formId
      ) {
        return;
      }
      setVersions(Array.isArray(rows) ? rows : []);
    }

    if ((!canWrite || activeTab === "preview") && formList.length > 0) {
      const entries = await Promise.all(
        formList.map(async (f) => {
          const rows = await transport.request<Version[]>(
            `/forms/${f.id}/versions`,
          );
          return [f.id, Array.isArray(rows) ? rows : []] as const;
        }),
      );
      if (contextRef.current !== contextKey) return;
      setAllFormVersions(Object.fromEntries(entries));
    }

    let activeBinding: Binding | undefined;
    if (selectedEventId) {
      const rows = await transport.request<Binding[]>(
        `/events/${selectedEventId}/form-bindings`,
      );
      if (
        contextRef.current !== contextKey ||
        selectionRef.current.selectedEventId !== selectedEventId
      ) {
        return;
      }
      const bindingList = Array.isArray(rows) ? rows : [];
      setBindings(bindingList);
      activeBinding =
        bindingList.find((b) => b.status === "open") || bindingList[0];
    }

    const previewTargetId =
      previewVersionId ||
      (canWrite && versionId) ||
      activeBinding?.form_version_id ||
      "";
    if (previewTargetId) {
      const detail = await transport.request<VersionDetail>(
        `/form-versions/${previewTargetId}`,
      );
      if (contextRef.current === contextKey) {
        setPreviewVersion(detail || null);
      }
    } else {
      setPreviewVersion(null);
    }
  }, [
    transport,
    formId,
    versionId,
    previewVersionId,
    activeTab,
    selectedEventId,
    canWrite,
    contextKey,
  ]);

  useEffect(() => {
    setForms([]);
    setVersions([]);
    setBindings([]);
    setFormId("");
    setVersionId("");
    setPreviewVersionId("");
    setBindingId("");
    setDraftEditor(null);
    setNotice("");
    setBusy(false);
    setName("");
    setPurpose("registration");
    setFieldName("nombre");
    setFieldTitle("Nombre");
    setFieldType("string");
    setExtraFields([]);
    setConsent("");
    setConsentVersion("");
    setPolicyPurpose("");
    setClassification("interna");
    setRetention("");
    setBindingStatus("open");
    setOpens("");
    setCloses("");
    setReason("");
  }, [contextKey]);

  useEffect(() => {
    setBindings([]);
    setBindingId("");
    setReason("");
  }, [selectedEventId]);

  useEffect(() => {
    let active = true;
    void load().catch((error) => {
      if (active) {
        setNotice(error instanceof Error ? error.message : String(error));
      }
    });
    return () => {
      active = false;
    };
  }, [load]);

  async function action(task: () => Promise<void>) {
    setBusy(true);
    setNotice("");
    setFormErrors({});
    try {
      await task();
      if (contextRef.current !== contextKey) return;
      await load();
      if (contextRef.current === contextKey) {
        setNotice(tr("Operación completada."));
      }
    } catch (error) {
      if (contextRef.current === contextKey) {
        if (
          error instanceof ApiError &&
          Object.keys(error.fieldErrors).length > 0
        ) {
          setFormErrors(error.fieldErrors);
        }
        setNotice(error instanceof Error ? error.message : String(error));
      }
    } finally {
      if (contextRef.current === contextKey) {
        setBusy(false);
      }
    }
  }

  async function createDefinition(event: FormEvent) {
    event.preventDefault();
    if (!name.trim()) {
      setFormErrors({ name: tr("El nombre del formulario es obligatorio.") });
      return;
    }
    await action(async () => {
      const row = await transport.request<Definition>("/forms", {
        method: "POST",
        body: { name, purpose },
      });
      if (contextRef.current === contextKey) {
        setFormId(row.id);
        setName("");
      }
    });
  }

  async function createDraft(event: FormEvent) {
    event.preventDefault();
    const errs: Record<string, string> = {};
    if (!fieldName.trim() || !/^[a-z][a-z0-9_]*$/.test(fieldName)) {
      errs.fieldName = tr(
        "Usa minúsculas, números o guion bajo (ej. nombre_completo).",
      );
    }
    if (!fieldTitle.trim())
      errs.fieldTitle = tr("La etiqueta del campo es obligatoria.");
    if (!consent.trim())
      errs.consent = tr("El texto de consentimiento es obligatorio.");
    if (!consentVersion.trim())
      errs.consentVersion = tr(
        "Indica la versión del consentimiento (ej. v1.0).",
      );
    if (!policyPurpose.trim())
      errs.policyPurpose = tr("La finalidad del campo es obligatoria.");
    if (!retention.trim())
      errs.retention = tr(
        "Indica la referencia de retención (ej. event-plus-90d).",
      );
    if (Object.keys(errs).length > 0) {
      setFormErrors(errs);
      return;
    }
    await action(async () => {
      const fields = [
        ...extraFields,
        {
          name: fieldName,
          title: fieldTitle,
          type: fieldType,
          purpose: policyPurpose,
          classification,
          retention,
        },
      ];
      if (new Set(fields.map((field) => field.name)).size !== fields.length) {
        throw new Error(tr("Los identificadores de campos deben ser únicos."));
      }
      const names = fields.map((field) => field.name);
      const properties = Object.fromEntries(
        fields.map((field) => [
          field.name,
          { type: field.type, title: field.title },
        ]),
      );
      const policies = Object.fromEntries(
        fields.map((field) => [
          field.name,
          {
            purpose: field.purpose,
            classification: field.classification,
            retention: field.retention,
            export_allowlist: [],
          },
        ]),
      );
      const body = {
        schema: {
          $schema: "https://json-schema.org/draft/2020-12/schema",
          type: "object",
          properties,
          required: names,
        },
        ui_schema: { order: names },
        consent: { text: consent, version: consentVersion },
        field_policies: policies,
      };
      const row = await transport.request<Version>(
        `/forms/${formId}/versions`,
        {
          method: "POST",
          body,
        },
      );
      if (contextRef.current === contextKey) {
        setVersionId(row.id);
      }
    });
  }

  function addField() {
    if (
      !/^[a-z][a-z0-9_]{0,63}$/.test(fieldName) ||
      !fieldTitle.trim() ||
      !policyPurpose.trim() ||
      !retention.trim()
    ) {
      setNotice(tr("Completa los datos del campo y su política."));
      return;
    }
    if (
      extraFields.some((field) => field.name === fieldName) ||
      fieldName === ""
    ) {
      setNotice(tr("Los identificadores de campos deben ser únicos."));
      return;
    }
    setExtraFields([
      ...extraFields,
      {
        name: fieldName,
        title: fieldTitle,
        type: fieldType,
        purpose: policyPurpose,
        classification,
        retention,
      },
    ]);
    setFieldName("");
    setFieldTitle("");
    setNotice("");
  }

  async function editDraft() {
    if (!selected || selected.status !== "draft") return;
    await action(async () => {
      const row = await transport.request<{
        id: string;
        revision: number;
        schema: unknown;
        ui_schema: unknown;
        consent: unknown;
        field_policies: unknown;
      }>(`/form-versions/${selected.id}`);
      if (
        contextRef.current !== contextKey ||
        selectionRef.current.formId !== formId
      ) {
        return;
      }
      setDraftEditor({
        id: row.id,
        revision: row.revision,
        schema: JSON.stringify(row.schema, null, 2),
        ui_schema: JSON.stringify(row.ui_schema, null, 2),
        consent: JSON.stringify(row.consent, null, 2),
        field_policies: JSON.stringify(row.field_policies, null, 2),
      });
    });
  }

  async function saveDraft(event: FormEvent) {
    event.preventDefault();
    if (!draftEditor) return;
    await action(async () => {
      const body = {
        schema: JSON.parse(draftEditor.schema),
        ui_schema: JSON.parse(draftEditor.ui_schema),
        consent: JSON.parse(draftEditor.consent),
        field_policies: JSON.parse(draftEditor.field_policies),
      };
      await transport.request(`/form-versions/${draftEditor.id}`, {
        method: "PATCH",
        body,
        revision: draftEditor.revision,
      });
      if (contextRef.current === contextKey) {
        setDraftEditor(null);
      }
    });
  }

  async function publish() {
    if (!selected) return;
    await action(async () => {
      const result = await transport.request<{
        valid: boolean;
        errors: string[];
        expected_hash: string;
      }>(`/form-versions/${selected.id}/validations`, { method: "POST" });
      if (
        contextRef.current !== contextKey ||
        selectionRef.current.formId !== formId
      ) {
        return;
      }
      if (!result.valid) {
        throw new Error(result.errors.join("; "));
      }
      await transport.request(`/form-versions/${selected.id}/publications`, {
        method: "POST",
        body: { expected_hash: result.expected_hash },
        revision: selected.revision,
      });
    });
  }

  async function bind() {
    if (!selectedEventId || !selected) return;
    await action(async () => {
      await transport.request(`/events/${selectedEventId}/form-bindings`, {
        method: "POST",
        body: {
          form_version_id: selected.id,
          purpose: forms.find((f) => f.id === formId)?.purpose,
          status: bindingStatus,
          opens_at: new Date(opens).toISOString(),
          closes_at: new Date(closes).toISOString(),
        },
      });
    });
  }

  async function transition(target: "open" | "cancel") {
    const row = bindings.find((b) => b.id === bindingId);
    if (!row || row.event_id !== selectedEventId) return;
    await action(async () => {
      await transport.request(
        `/bindings/${row.id}/${target === "open" ? "openings" : "cancellations"}`,
        {
          method: "POST",
          body: target === "cancel" ? { reason } : undefined,
          revision: row.revision,
        },
      );
    });
  }

  async function close() {
    const row = bindings.find((b) => b.id === bindingId);
    if (!row || row.event_id !== selectedEventId) return;
    await action(async () => {
      await transport.request(`/bindings/${row.id}/closures`, {
        method: "POST",
        body: { reason },
        revision: row.revision,
      });
    });
  }

  const activeBinding =
    bindings.find((b) => b.form_version_id === previewVersion?.id) ||
    bindings.find((b) => b.status === "open") ||
    bindings[0];

  const selectedPreviewId =
    previewVersionId ||
    previewVersion?.id ||
    activeBinding?.form_version_id ||
    "";

  const previewSelector = (
    <label className="field">
      {tr("Formulario y versión")}
      <select
        value={selectedPreviewId}
        onChange={(e) => setPreviewVersionId(e.target.value)}
      >
        <option value="">{tr("Selecciona una opción")}</option>
        {bindings.length > 0 && (
          <optgroup label={tr("Vinculados a este evento")}>
            {bindings.map((b) => {
              const vMatch = Object.entries(allFormVersions).find(([, vs]) =>
                vs.some((v) => v.id === b.form_version_id),
              );
              const formDef = vMatch
                ? forms.find((f) => f.id === vMatch[0])
                : undefined;
              const ver = vMatch?.[1].find((v) => v.id === b.form_version_id);
              return (
                <option key={`bind-${b.id}`} value={b.form_version_id || ""}>
                  {formDef ? `${formDef.name} · ` : ""}
                  {ver ? `v${ver.version_number} · ${ver.status}` : b.purpose} (
                  {tr("Vínculo")}: {b.status})
                </option>
              );
            })}
          </optgroup>
        )}
        {forms.map((f) => {
          const vs = allFormVersions[f.id] || [];
          if (vs.length === 0) return null;
          return (
            <optgroup key={f.id} label={`${f.name} · ${f.purpose}`}>
              {vs.map((v) => {
                const bound = bindings.find((b) => b.form_version_id === v.id);
                return (
                  <option key={v.id} value={v.id}>
                    v{v.version_number} · {v.status}
                    {bound ? ` (${tr("Vinculado")}: ${bound.status})` : ""}
                  </option>
                );
              })}
            </optgroup>
          );
        })}
      </select>
    </label>
  );

  if (!canWrite) {
    return (
      <div className="stack">
        <Panel
          title={tr("Formulario del evento")}
          subtitle={tr("Sin acceso de escritura")}
        >
          {previewSelector}
          <FormPreview
            version={previewVersion}
            binding={activeBinding}
            selectedEventId={selectedEventId}
          />
        </Panel>
      </div>
    );
  }

  return (
    <div className="stack">
      <Panel
        title={tr("Formularios y vínculos")}
        subtitle={tr("Diseña versiones y asócialas a un evento.")}
      >
        <div
          className="subtabs"
          role="tablist"
          aria-label={tr("Vistas de formularios")}
        >
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "design"}
            className={`subtab ${activeTab === "design" ? "is-active" : ""}`}
            onClick={() => setActiveTab("design")}
          >
            {tr("Diseñar y vincular")}
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={activeTab === "preview"}
            className={`subtab ${activeTab === "preview" ? "is-active" : ""}`}
            onClick={() => setActiveTab("preview")}
          >
            {tr("Vista previa")}
          </button>
        </div>

        {notice && <p role="status">{notice}</p>}
        {activeTab === "preview" ? (
          <div className="stack">
            {previewSelector}
            <FormPreview
              version={previewVersion}
              binding={activeBinding}
              selectedEventId={selectedEventId}
            />
          </div>
        ) : (
          <div className="stack">
            <section className="subpanel">
              <div className="subpanel-head">
                <h3>{tr("Definición de formulario")}</h3>
                <p>
                  {tr(
                    "Crea un formulario base indicando su nombre y propósito.",
                  )}
                </p>
              </div>
              <form
                onSubmit={createDefinition}
                noValidate
                className="form-grid"
              >
                <label
                  className={`field ${formErrors.name ? "has-error" : ""}`}
                >
                  {tr("Nombre del formulario")}
                  <input
                    required
                    value={name}
                    aria-invalid={Boolean(formErrors.name)}
                    onChange={(e) => {
                      setFormErrors((prev) => ({ ...prev, name: "" }));
                      setName(e.target.value);
                    }}
                  />
                  {formErrors.name && (
                    <span className="field-error" role="alert">
                      <AlertCircle size={14} /> {formErrors.name}
                    </span>
                  )}
                </label>
                <label className="field">
                  {tr("Propósito")}
                  <select
                    value={purpose}
                    onChange={(e) => setPurpose(e.target.value)}
                  >
                    <option value="registration">Registration</option>
                    <option value="application">Application</option>
                    <option value="feedback">Feedback</option>
                  </select>
                </label>
                <div className="form-actions">
                  <button className="button primary" disabled={busy}>
                    {tr("Crear formulario")}
                  </button>
                </div>
              </form>
            </section>

            <section className="subpanel">
              <div className="subpanel-head">
                <h3>{tr("Versiones y campos")}</h3>
                <p>
                  {tr(
                    "Selecciona un formulario existente para diseñar o publicar una versión.",
                  )}
                </p>
              </div>
              <label className="field">
                {tr("Formulario")}
                <select
                  value={formId}
                  onChange={(e) => {
                    setFormId(e.target.value);
                    setVersionId("");
                    setDraftEditor(null);
                  }}
                >
                  <option value="">{tr("Selecciona una opción")}</option>
                  {forms.map((f) => (
                    <option key={f.id} value={f.id}>
                      {f.name} · {f.purpose}
                    </option>
                  ))}
                </select>
              </label>

              {canWrite && formId && (
                <form onSubmit={createDraft} noValidate className="form-grid">
                  <label
                    className={`field ${formErrors.fieldName ? "has-error" : ""}`}
                  >
                    {tr("Identificador del campo")}
                    <input
                      required
                      pattern="[a-z][a-z0-9_]*"
                      value={fieldName}
                      aria-invalid={Boolean(formErrors.fieldName)}
                      onChange={(e) => {
                        setFormErrors((prev) => ({ ...prev, fieldName: "" }));
                        setFieldName(e.target.value);
                      }}
                    />
                    {formErrors.fieldName && (
                      <span className="field-error" role="alert">
                        <AlertCircle size={14} /> {formErrors.fieldName}
                      </span>
                    )}
                  </label>
                  <label
                    className={`field ${formErrors.fieldTitle ? "has-error" : ""}`}
                  >
                    {tr("Etiqueta del campo")}
                    <input
                      required
                      value={fieldTitle}
                      aria-invalid={Boolean(formErrors.fieldTitle)}
                      onChange={(e) => {
                        setFormErrors((prev) => ({ ...prev, fieldTitle: "" }));
                        setFieldTitle(e.target.value);
                      }}
                    />
                    {formErrors.fieldTitle && (
                      <span className="field-error" role="alert">
                        <AlertCircle size={14} /> {formErrors.fieldTitle}
                      </span>
                    )}
                  </label>
                  <label className="field">
                    {tr("Tipo de campo")}
                    <select
                      value={fieldType}
                      onChange={(e) => setFieldType(e.target.value)}
                    >
                      <option value="string">Texto</option>
                      <option value="number">Número</option>
                      <option value="integer">Entero</option>
                      <option value="boolean">Sí/No</option>
                    </select>
                  </label>
                  <label
                    className={`field ${formErrors.consent ? "has-error" : ""}`}
                  >
                    {tr("Texto de consentimiento")}
                    <textarea
                      required
                      value={consent}
                      aria-invalid={Boolean(formErrors.consent)}
                      onChange={(e) => {
                        setFormErrors((prev) => ({ ...prev, consent: "" }));
                        setConsent(e.target.value);
                      }}
                    />
                    {formErrors.consent && (
                      <span className="field-error" role="alert">
                        <AlertCircle size={14} /> {formErrors.consent}
                      </span>
                    )}
                  </label>
                  <label
                    className={`field ${formErrors.consentVersion ? "has-error" : ""}`}
                  >
                    {tr("Versión del consentimiento")}
                    <input
                      required
                      value={consentVersion}
                      aria-invalid={Boolean(formErrors.consentVersion)}
                      onChange={(e) => {
                        setFormErrors((prev) => ({
                          ...prev,
                          consentVersion: "",
                        }));
                        setConsentVersion(e.target.value);
                      }}
                    />
                    {formErrors.consentVersion && (
                      <span className="field-error" role="alert">
                        <AlertCircle size={14} /> {formErrors.consentVersion}
                      </span>
                    )}
                  </label>
                  <label
                    className={`field ${formErrors.policyPurpose ? "has-error" : ""}`}
                  >
                    {tr("Finalidad del campo")}
                    <input
                      required
                      value={policyPurpose}
                      aria-invalid={Boolean(formErrors.policyPurpose)}
                      onChange={(e) => {
                        setFormErrors((prev) => ({
                          ...prev,
                          policyPurpose: "",
                        }));
                        setPolicyPurpose(e.target.value);
                      }}
                    />
                    {formErrors.policyPurpose && (
                      <span className="field-error" role="alert">
                        <AlertCircle size={14} /> {formErrors.policyPurpose}
                      </span>
                    )}
                  </label>
                  <label className="field">
                    {tr("Clasificación")}
                    <select
                      value={classification}
                      onChange={(e) => setClassification(e.target.value)}
                    >
                      {[
                        "publica",
                        "interna",
                        "confidencial",
                        "restringida",
                      ].map((x) => (
                        <option key={x}>{x}</option>
                      ))}
                    </select>
                    {["confidencial", "restringida"].includes(
                      classification,
                    ) && (
                      <small role="status">
                        {tr(
                          "Campo de riesgo: revisa la política y el consentimiento.",
                        )}
                      </small>
                    )}
                  </label>
                  <label
                    className={`field ${formErrors.retention ? "has-error" : ""}`}
                  >
                    {tr("Referencia de retención")}
                    <input
                      required
                      value={retention}
                      aria-invalid={Boolean(formErrors.retention)}
                      onChange={(e) => {
                        setFormErrors((prev) => ({ ...prev, retention: "" }));
                        setRetention(e.target.value);
                      }}
                    />
                    {formErrors.retention && (
                      <span className="field-error" role="alert">
                        <AlertCircle size={14} /> {formErrors.retention}
                      </span>
                    )}
                  </label>
                  <div className="field wide">
                    <strong>{tr("Campos incluidos en el borrador")}</strong>
                    {extraFields.map((field, index) => (
                      <small key={field.name}>
                        {index + 1}. {field.title} · {field.name} ({field.type})
                      </small>
                    ))}
                    {fieldName && fieldTitle && (
                      <small>
                        {extraFields.length + 1}. {fieldTitle} · {fieldName} (
                        {fieldType})
                      </small>
                    )}
                  </div>
                  <div className="form-actions">
                    <button className="button primary" disabled={busy}>
                      {tr("Crear versión borrador")}
                    </button>
                    <button
                      type="button"
                      className="button secondary"
                      onClick={addField}
                    >
                      {tr("Agregar campo a la vista previa")}
                    </button>
                  </div>
                </form>
              )}

              {formId && (
                <>
                  <label className="field">
                    {tr("Versión")}
                    <select
                      aria-label={tr("Versión")}
                      value={versionId}
                      onChange={(e) => {
                        setVersionId(e.target.value);
                        setDraftEditor(null);
                      }}
                    >
                      <option value="">{tr("Selecciona una opción")}</option>
                      {versions.map((v) => (
                        <option key={v.id} value={v.id}>
                          v{v.version_number} · {v.status}
                        </option>
                      ))}
                    </select>
                  </label>
                  {canWrite && selected?.status === "draft" && (
                    <button
                      type="button"
                      className="button secondary"
                      disabled={busy}
                      onClick={() => void editDraft()}
                    >
                      {tr("Editar borrador")}
                    </button>
                  )}
                  {selected?.status === "draft" && canPublish && (
                    <button
                      type="button"
                      className="button primary"
                      disabled={busy}
                      onClick={() => void publish()}
                    >
                      {tr("Validar y publicar formulario")}
                    </button>
                  )}
                </>
              )}

              {draftEditor && (
                <form onSubmit={saveDraft} className="form-grid">
                  {(
                    [
                      "schema",
                      "ui_schema",
                      "consent",
                      "field_policies",
                    ] as const
                  ).map((key) => (
                    <label className="field" key={key}>
                      {key}
                      <textarea
                        aria-label={key}
                        rows={7}
                        required
                        value={draftEditor[key]}
                        onChange={(e) =>
                          setDraftEditor({
                            ...draftEditor,
                            [key]: e.target.value,
                          })
                        }
                      />
                    </label>
                  ))}
                  <div className="form-actions">
                    <button className="button primary" disabled={busy}>
                      {tr("Guardar borrador")}
                    </button>
                  </div>
                </form>
              )}
            </section>

            {selectedEventId &&
              (selected?.status === "published" || bindings.length > 0) && (
                <section className="subpanel">
                  <div className="subpanel-head">
                    <h3>{tr("Vínculo con el evento")}</h3>
                    <p>
                      {tr(
                        "Asocia una versión publicada al evento de trabajo o gestiona los vínculos activos.",
                      )}
                    </p>
                  </div>

                  {canWrite &&
                    selected?.status === "published" &&
                    selectedEventId && (
                      <div className="form-grid">
                        <label className="field">
                          {tr("Estado inicial")}
                          <select
                            value={bindingStatus}
                            onChange={(e) => setBindingStatus(e.target.value)}
                          >
                            <option value="open">{tr("Abierto")}</option>
                            <option value="scheduled">
                              {tr("Programado")}
                            </option>
                          </select>
                        </label>
                        <label className="field">
                          {tr("Apertura")}
                          <input
                            type="datetime-local"
                            value={opens}
                            onChange={(e) => setOpens(e.target.value)}
                          />
                        </label>
                        <label className="field">
                          {tr("Cierre")}
                          <input
                            type="datetime-local"
                            value={closes}
                            onChange={(e) => setCloses(e.target.value)}
                          />
                        </label>
                        <div className="form-actions">
                          <button
                            className="button secondary"
                            disabled={busy || !opens || !closes}
                            onClick={() => void bind()}
                          >
                            {tr("Vincular al evento")}
                          </button>
                        </div>
                      </div>
                    )}

                  {selectedEventId && bindings.length > 0 && (
                    <div className="form-grid">
                      <label className="field">
                        {tr("Vínculo")}
                        <select
                          value={bindingId}
                          onChange={(e) => setBindingId(e.target.value)}
                        >
                          <option value="">
                            {tr("Selecciona una opción")}
                          </option>
                          {bindings
                            .filter(
                              (b) =>
                                b.status === "open" || b.status === "scheduled",
                            )
                            .map((b) => (
                              <option key={b.id} value={b.id}>
                                {b.purpose} · {b.status} · {b.id}
                              </option>
                            ))}
                        </select>
                      </label>
                      {canWrite && (
                        <>
                          <label className="field">
                            {tr("Motivo")}
                            <input
                              value={reason}
                              onChange={(e) => setReason(e.target.value)}
                            />
                          </label>
                          {bindings.find((b) => b.id === bindingId)?.status ===
                            "scheduled" && (
                            <button
                              className="button secondary"
                              disabled={busy}
                              onClick={() => void transition("open")}
                            >
                              {tr("Abrir vínculo")}
                            </button>
                          )}
                          {bindings.find((b) => b.id === bindingId)?.status ===
                            "open" && (
                            <button
                              className="button secondary"
                              disabled={busy || !reason.trim()}
                              onClick={() => void close()}
                            >
                              {tr("Cerrar vínculo")}
                            </button>
                          )}
                          <button
                            className="button secondary"
                            disabled={busy || !bindingId || !reason.trim()}
                            onClick={() => void transition("cancel")}
                          >
                            {tr("Cancelar vínculo")}
                          </button>
                        </>
                      )}
                    </div>
                  )}
                </section>
              )}
          </div>
        )}
      </Panel>
    </div>
  );
}
