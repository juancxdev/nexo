// Package reports genera los archivos de reporte a partir de los contratos de lectura
// de Onsite (onsite_contract.*). Nunca lee tablas internas de otro servicio.
package reports

import (
	"bytes"
	"context"
	"encoding/csv"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
)

var Types = map[string]bool{"attendance": true, "presence": true, "rejections": true, "summary": true}

var ErrUnknownType = errors.New("unknown_report_type")

// Querier es lo mínimo que se necesita de pgx (pool o transacción).
type Querier interface {
	Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error)
}

type Table struct {
	Header []string
	Rows   [][]string
}

// Build arma la tabla del reporte. scope vacío = todas las zonas.
func Build(ctx context.Context, q Querier, kind, tenant, event, scope string) (Table, error) {
	var sql string
	var header []string
	switch kind {
	case "attendance":
		header = []string{"fact_id", "ticket_id", "titular", "zona", "tipo", "fecha_hora", "anulado"}
		sql = `SELECT fact_id::text,ticket_id,masked_holder,scope,type,to_char(occurred_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"'),CASE WHEN voided THEN 'si' ELSE 'no' END
			FROM onsite_contract.attendance_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3) ORDER BY occurred_at,fact_id`
	case "presence":
		header = []string{"ticket_id", "titular", "zona", "ingreso"}
		sql = `SELECT ticket_id,masked_holder,scope,to_char(checked_in_at AT TIME ZONE 'UTC','YYYY-MM-DD"T"HH24:MI:SS"Z"')
			FROM onsite_contract.presence_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3) ORDER BY checked_in_at,ticket_id`
	case "rejections":
		header = []string{"motivo", "zona", "cantidad"}
		sql = `SELECT reason,scope,COUNT(*)::text FROM onsite_contract.scan_rejections_v1
			WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3) GROUP BY reason,scope ORDER BY COUNT(*) DESC,reason`
	case "summary":
		header = []string{"indicador", "valor"}
		sql = `SELECT k,v::text FROM (
			SELECT 1 o,'entradas_vigentes' k,COUNT(*) v FROM onsite_contract.attendance_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3) AND type='check_in' AND NOT voided
			UNION ALL SELECT 2,'salidas_vigentes',COUNT(*) FROM onsite_contract.attendance_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3) AND type='check_out' AND NOT voided
			UNION ALL SELECT 3,'registros_anulados',COUNT(*) FROM onsite_contract.attendance_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3) AND voided
			UNION ALL SELECT 4,'personas_unicas',COUNT(DISTINCT ticket_id) FROM onsite_contract.attendance_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3) AND type='check_in' AND NOT voided
			UNION ALL SELECT 5,'presentes_ahora',COUNT(*) FROM onsite_contract.presence_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3)
			UNION ALL SELECT 6,'rechazos_qr',COUNT(*) FROM onsite_contract.scan_rejections_v1 WHERE tenant_id=$1::uuid AND event_id=$2 AND ($3='' OR scope=$3)
		) x ORDER BY o`
	default:
		return Table{}, ErrUnknownType
	}
	rows, err := q.Query(ctx, sql, tenant, event, scope)
	if err != nil {
		return Table{}, err
	}
	defer rows.Close()
	t := Table{Header: header, Rows: [][]string{}}
	for rows.Next() {
		vals := make([]string, len(header))
		ptrs := make([]any, len(header))
		for i := range vals {
			ptrs[i] = &vals[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Table{}, err
		}
		t.Rows = append(t.Rows, vals)
	}
	return t, rows.Err()
}

// safeCell evita la inyección de fórmulas al abrir el CSV en Excel o Sheets.
func safeCell(v string) string {
	if v != "" && strings.ContainsAny(v[:1], "=+-@\t\r") {
		return "'" + v
	}
	return v
}

// Encode serializa la tabla. CSV lleva BOM UTF-8 para que Excel muestre bien las tildes.
func Encode(t Table, format, kind, event string, generated time.Time) ([]byte, string, error) {
	var buf bytes.Buffer
	switch format {
	case "csv":
		buf.WriteString("\xEF\xBB\xBF")
		w := csv.NewWriter(&buf)
		_ = w.Write(t.Header)
		for _, r := range t.Rows {
			out := make([]string, len(r))
			for i, v := range r {
				out[i] = safeCell(v)
			}
			_ = w.Write(out)
		}
		w.Flush()
		return buf.Bytes(), "text/csv; charset=utf-8", w.Error()
	case "json":
		items := make([]map[string]string, 0, len(t.Rows))
		for _, r := range t.Rows {
			m := map[string]string{}
			for i, h := range t.Header {
				m[h] = r[i]
			}
			items = append(items, m)
		}
		raw, err := json.Marshal(map[string]any{"report_type": kind, "event_id": event, "generated_at": generated.UTC().Format(time.RFC3339), "row_count": len(items), "items": items})
		return raw, "application/json", err
	}
	return nil, "", errors.New("invalid_format")
}
