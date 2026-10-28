// Package governance is the SQL boundary for Registry governance.
package governance

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	g "github.com/agentregistry-dev/agentregistry/internal/registry/governance"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/v1alpha1store"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

const table = "agentregistry.governance_records"
const auditTable = "agentregistry.governance_audit"
const columns = `id::text,kind,department,owner_id,state,version,COALESCE(parent_id::text,''),COALESCE(tool_id::text,''),COALESCE(source_id::text,''),data,hash,decision_by,decided_at,reason,created_at,updated_at,revoked_at,revoked_by`

type Store struct{ pool *pgxpool.Pool }

func New(pool *pgxpool.Pool) *Store { return &Store{pool: pool} }

type unit struct {
	tx    pgx.Tx
	actor g.Actor
}

func mapError(e error) error {
	if errors.Is(e, pgx.ErrNoRows) {
		return g.ErrNotFound
	}
	var p *pgconn.PgError
	if errors.As(e, &p) && (p.Code == "23505" || p.Code == "40001" || p.Code == "40P01") {
		return g.ErrConflict
	}
	if e != nil {
		return fmt.Errorf("governance persistence: %w", e)
	}
	return nil
}
func (s *Store) Within(ctx context.Context, a g.Actor, fn func(g.Unit) error) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return mapError(e)
	}
	defer tx.Rollback(ctx)
	// A department lock serializes decisions with suspension/revocation and avoids
	// lock-order deadlocks. It is database-backed and safe across server replicas.
	if _, e = tx.Exec(ctx, `SELECT pg_advisory_xact_lock(hashtextextended($1,17917))`, a.Department); e != nil {
		return mapError(e)
	}
	if a.Revision > 0 {
		var revision int
		if e = tx.QueryRow(ctx, `SELECT revision FROM agentregistry.governance_accounts WHERE id=$1 AND department=$2`, a.ID, a.Department).Scan(&revision); e != nil {
			return mapError(e)
		}
		if revision != a.Revision {
			return g.ErrForbidden
		}
	}
	if e = fn(&unit{tx: tx, actor: a}); e != nil {
		return e
	}
	return mapError(tx.Commit(ctx))
}
func scan(row pgx.Row) (g.Record, error) {
	var r g.Record
	var data []byte
	e := row.Scan(&r.ID, &r.Kind, &r.Department, &r.Owner, &r.State, &r.Version, &r.Parent, &r.Tool, &r.Source, &data, &r.Hash, &r.DecisionBy, &r.DecidedAt, &r.Reason, &r.CreatedAt, &r.UpdatedAt, &r.RevokedAt, &r.RevokedBy)
	if e != nil {
		return r, mapError(e)
	}
	r.CreatedAt = r.CreatedAt.UTC()
	r.UpdatedAt = r.UpdatedAt.UTC()
	if r.DecidedAt != nil {
		v := r.DecidedAt.UTC()
		r.DecidedAt = &v
	}
	if r.RevokedAt != nil {
		v := r.RevokedAt.UTC()
		r.RevokedAt = &v
	}
	e = json.Unmarshal(data, &r.Data)
	return r, mapError(e)
}
func (u *unit) Get(ctx context.Context, id string) (g.Record, error) {
	return scan(u.tx.QueryRow(ctx, `SELECT `+columns+` FROM `+table+` WHERE id::text=$1 AND department=$2 FOR UPDATE`, id, u.actor.Department))
}
func (u *unit) Save(ctx context.Context, r g.Record, create bool) error {
	if r.Department != u.actor.Department {
		return g.ErrForbidden
	}
	b, e := json.Marshal(r.Data)
	if e != nil {
		return e
	}
	if create {
		_, e = u.tx.Exec(ctx, `INSERT INTO `+table+` (id,kind,department,owner_id,state,version,parent_id,tool_id,source_id,agent_uid,server_uid,data,hash,decision_by,decided_at,reason,created_at,updated_at)
   VALUES($1,$2,$3,$4,$5,$6,NULLIF($7,'')::uuid,NULLIF($8,'')::uuid,NULLIF($9,'')::uuid,
    (SELECT uid FROM agentregistry.agents WHERE $2='agent' AND namespace=$10 AND name=$11 AND tag=$12),
    (SELECT uid FROM agentregistry.mcp_servers WHERE $2='server' AND namespace=$10 AND name=$11 AND tag=$12),$13,$14,$15,$16,$17,$18,$19)`, r.ID, r.Kind, r.Department, r.Owner, r.State, r.Version, r.Parent, r.Tool, r.Source, r.Data.Namespace, r.Data.Name, r.Data.Tag, b, r.Hash, r.DecisionBy, r.DecidedAt, r.Reason, r.CreatedAt, r.UpdatedAt)
	} else {
		var tag pgconn.CommandTag
		tag, e = u.tx.Exec(ctx, `UPDATE `+table+` SET state=$1,version=$2,data=$3,hash=$4,decision_by=$5,decided_at=$6,reason=$7,updated_at=$8,revoked_at=$12,revoked_by=$13 WHERE id=$9 AND department=$10 AND version=$11`, r.State, r.Version, b, r.Hash, r.DecisionBy, r.DecidedAt, r.Reason, r.UpdatedAt, r.ID, u.actor.Department, r.Version-1, r.RevokedAt, r.RevokedBy)
		if e == nil && tag.RowsAffected() != 1 {
			return g.ErrConflict
		}
	}
	return mapError(e)
}
func (u *unit) Catalog(ctx context.Context, r g.Record) error {
	t := "agents"
	if r.Kind == "server" {
		t = "mcp_servers"
	}
	spec := map[string]any{"title": r.Data.Name, "description": r.Data.Description}
	if r.Kind == "server" && r.Data.Endpoint != "" {
		spec["remote"] = map[string]string{"type": "streamable-http", "url": r.Data.Endpoint}
	}
	b, _ := json.Marshal(spec)
	hash, err := v1alpha1store.ContentHash(nil, b)
	if err != nil {
		return mapError(err)
	}
	// Existing catalog entries are never silently overwritten or claimed.
	_, e := u.tx.Exec(ctx, `INSERT INTO agentregistry.`+t+`(namespace,name,tag,spec,content_hash) VALUES($1,$2,$3,$4,$5)`, r.Data.Namespace, r.Data.Name, r.Data.Tag, b, hash)
	return mapError(e)
}
func insertAudit(ctx context.Context, exec interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
}, e g.Event) error {
	_, err := exec.Exec(ctx, `INSERT INTO `+auditTable+`(id,at,actor,action,object_id,parent_id,department,revision,reason,result,correlation,before_hash,after_hash) VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)`, e.ID, e.At, e.Actor, e.Action, e.Object, e.Parent, e.Department, e.Revision, e.Reason, e.Result, e.Correlation, e.BeforeHash, e.AfterHash)
	return mapError(err)
}
func (u *unit) Audit(ctx context.Context, e g.Event) error   { return insertAudit(ctx, u.tx, e) }
func (s *Store) Denied(ctx context.Context, e g.Event) error { return insertAudit(ctx, s.pool, e) }
func pagination(f g.Filter) (int, int) {
	limit := f.Limit
	if limit < 1 {
		limit = 25
	}
	if limit > 100 {
		limit = 100
	}
	offset := f.Offset
	if offset < 0 {
		offset = 0
	}
	return limit, offset
}
func (s *Store) List(ctx context.Context, a g.Actor, f g.Filter, now time.Time) (g.Page, error) {
	p := g.Page{Items: []g.Record{}}
	limit, offset := pagination(f)
	// The same predicate feeds count and rows in a repeatable-read transaction.
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return p, mapError(e)
	}
	defer tx.Rollback(ctx)
	effective := `CASE WHEN kind='grant' AND state='approved' AND (data->>'expiresAt')::timestamptz <= $6 THEN 'expired' ELSE state END`
	where := ` FROM ` + table + ` WHERE department=$1 AND ($2='' OR kind=$2) AND ($3='' OR parent_id::text=$3 OR id::text=$3) AND ($4='' OR ` + effective + `=$4) AND ($5='' OR data->>'name' ILIKE '%'||$5||'%' OR id::text=$5)`
	args := []any{a.Department, f.Kind, f.Parent, f.State, f.Search, now}
	if e = tx.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&p.Total); e != nil {
		return p, mapError(e)
	}
	rows, e := tx.Query(ctx, `SELECT `+columns+where+` ORDER BY created_at,id LIMIT $7 OFFSET $8`, append(args, limit, offset)...)
	if e != nil {
		return p, mapError(e)
	}
	defer rows.Close()
	for rows.Next() {
		r, e := scan(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, g.Effective(r, now))
	}
	if e = rows.Err(); e != nil {
		return p, mapError(e)
	}
	return p, mapError(tx.Commit(ctx))
}
func (s *Store) Events(ctx context.Context, a g.Actor, f g.Filter) (g.Events, error) {
	p := g.Events{Items: []g.Event{}}
	limit, offset := pagination(f)
	tx, e := s.pool.BeginTx(ctx, pgx.TxOptions{IsoLevel: pgx.RepeatableRead, AccessMode: pgx.ReadOnly})
	if e != nil {
		return p, mapError(e)
	}
	defer tx.Rollback(ctx)
	where := ` FROM ` + auditTable + ` WHERE department=$1 AND ($2='' OR object_id IN (WITH RECURSIVE descendants AS (SELECT id FROM ` + table + ` WHERE id::text=$2 AND department=$1 UNION ALL SELECT r.id FROM ` + table + ` r JOIN descendants d ON r.parent_id=d.id WHERE r.department=$1) SELECT id::text FROM descendants) OR parent_id=$2) AND ($3='' OR actor=$3) AND ($4='' OR action=$4) AND ($5='' OR result=$5) AND ($6='' OR at>=NULLIF($6,'')::timestamptz) AND ($7='' OR at<=NULLIF($7,'')::timestamptz)`
	args := []any{a.Department, f.Parent, f.Actor, f.Action, f.State, f.From, f.Until}
	if e = tx.QueryRow(ctx, `SELECT count(*)`+where, args...).Scan(&p.Total); e != nil {
		return p, mapError(e)
	}
	rows, e := tx.Query(ctx, `SELECT id::text,at,actor,action,object_id,parent_id,department,revision,reason,result,correlation,before_hash,after_hash`+where+` ORDER BY at DESC,id LIMIT $8 OFFSET $9`, append(args, limit, offset)...)
	if e != nil {
		return p, mapError(e)
	}
	defer rows.Close()
	for rows.Next() {
		var v g.Event
		if e = rows.Scan(&v.ID, &v.At, &v.Actor, &v.Action, &v.Object, &v.Parent, &v.Department, &v.Revision, &v.Reason, &v.Result, &v.Correlation, &v.BeforeHash, &v.AfterHash); e != nil {
			return p, mapError(e)
		}
		v.At = v.At.UTC()
		p.Items = append(p.Items, v)
	}
	if e = rows.Err(); e != nil {
		return p, mapError(e)
	}
	return p, mapError(tx.Commit(ctx))
}
