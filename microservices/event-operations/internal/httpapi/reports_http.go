package httpapi

import (
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/jackc/pgx/v5"
)

type reportView struct {
	ID         string    `json:"id"`
	EventID    string    `json:"event_id"`
	Scope      string    `json:"scope"`
	Type       string    `json:"report_type"`
	Format     string    `json:"format"`
	Status     string    `json:"status"`
	Progress   int       `json:"progress"`
	RowCount   *int      `json:"row_count"`
	SHA256     *string   `json:"artifact_sha256"`
	ErrorCode  *string   `json:"error_code"`
	Attempts   int       `json:"attempts"`
	Downloaded bool      `json:"download_available"`
	CreatedAt  time.Time `json:"created_at"`
	ExpiresAt  time.Time `json:"expires_at"`
}

const reportSelect = `SELECT id::text,event_id,scope,report_type,format,status,progress,row_count,artifact_sha256,error_code,attempts,artifact IS NOT NULL,created_at,expires_at FROM operations.report_jobs`

func scanReports(rows pgx.Rows) ([]reportView, error) {
	defer rows.Close()
	out := []reportView{}
	for rows.Next() {
		var r reportView
		if err := rows.Scan(&r.ID, &r.EventID, &r.Scope, &r.Type, &r.Format, &r.Status, &r.Progress, &r.RowCount, &r.SHA256, &r.ErrorCode, &r.Attempts, &r.Downloaded, &r.CreatedAt, &r.ExpiresAt); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *Server) listReports(c fiber.Ctx) error {
	rows, err := s.DB.Query(c.Context(), reportSelect+` WHERE tenant_id=$1::uuid AND ($2='' OR event_id=$2) ORDER BY created_at DESC LIMIT 50`, tenant(c), c.Query("event_id"))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudieron leer los reportes")
	}
	items, err := scanReports(rows)
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudieron leer los reportes")
	}
	return c.JSON(fiber.Map{"items": items})
}

// downloadReport entrega el archivo generado. 409 si aún no está listo, 410 si se purgó.
func (s *Server) downloadReport(c fiber.Ctx) error {
	var status, format, kind, event string
	var content []byte
	var contentType *string
	err := s.DB.QueryRow(c.Context(), `SELECT status,format,report_type,event_id,artifact,content_type FROM operations.report_jobs WHERE id=$1::uuid AND tenant_id=$2::uuid`, c.Params("id"), tenant(c)).Scan(&status, &format, &kind, &event, &content, &contentType)
	if err != nil {
		return problem(c, 404, "report_not_found", "reporte no encontrado")
	}
	if status == "expired" || status == "purged" {
		return problem(c, 410, "report_purged", "el archivo venció y fue eliminado; vuelve a generarlo")
	}
	if status != "succeeded" || content == nil {
		return problem(c, 409, "report_not_ready", "el reporte aún no está listo")
	}
	ct := "application/octet-stream"
	if contentType != nil {
		ct = *contentType
	}
	c.Set("Content-Type", ct)
	c.Set("Content-Disposition", `attachment; filename="reporte-`+kind+`-`+event+`.`+format+`"`)
	c.Set("Cache-Control", "no-store")
	return c.Send(content)
}

func (s *Server) retryReport(c fiber.Ctx) error {
	tag, err := s.DB.Exec(c.Context(), `UPDATE operations.report_jobs SET status='pending',progress=0,error_code=NULL,artifact=NULL,lease_until=NULL,expires_at=now()+interval '1 day',updated_at=now()
		WHERE id=$1::uuid AND tenant_id=$2::uuid AND status IN ('failed','cancelled','expired','purged')`, c.Params("id"), tenant(c))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo reintentar")
	}
	if tag.RowsAffected() == 0 {
		return problem(c, 409, "invalid_state", "sólo se reintenta un reporte fallido, cancelado o vencido")
	}
	return c.Status(202).JSON(fiber.Map{"id": c.Params("id"), "status": "pending"})
}

func (s *Server) purgeReport(c fiber.Ctx) error {
	tag, err := s.DB.Exec(c.Context(), `UPDATE operations.report_jobs SET status='purged',artifact=NULL,updated_at=now() WHERE id=$1::uuid AND tenant_id=$2::uuid AND status IN ('succeeded','failed','cancelled','expired')`, c.Params("id"), tenant(c))
	if err != nil {
		return problem(c, 503, "persistence_unavailable", "no se pudo purgar")
	}
	if tag.RowsAffected() == 0 {
		return problem(c, 409, "invalid_state", "el reporte no se puede purgar en su estado actual")
	}
	return c.SendStatus(204)
}
