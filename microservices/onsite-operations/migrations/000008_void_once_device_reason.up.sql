-- Aportes sobre la base de Juan:
-- 1) un registro de asistencia sólo se puede anular una vez (también ante dos supervisores a la vez);
-- 2) la revocación de un dispositivo guarda su motivo.
CREATE UNIQUE INDEX IF NOT EXISTS attendance_single_void ON onsite.attendance_facts(compensates_fact_id) WHERE type = 'void';
ALTER TABLE onsite.devices ADD COLUMN IF NOT EXISTS revoked_reason text;
