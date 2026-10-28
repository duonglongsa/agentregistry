package governance

import (
	"context"
	"encoding/json"
	"time"

	g "github.com/agentregistry-dev/agentregistry/internal/registry/governance"
	"github.com/google/uuid"
)

func (s *Store) Provision(ctx context.Context, accounts []g.Account) error {
	tx, e := s.pool.Begin(ctx)
	if e != nil {
		return mapError(e)
	}
	defer tx.Rollback(ctx)
	for _, a := range accounts {
		roles, _ := json.Marshal(a.Roles)
		tag, e := tx.Exec(ctx, `INSERT INTO agentregistry.governance_accounts(id,name,department,roles,password_hash) VALUES($1,$2,$3,$4,$5) ON CONFLICT(id) DO NOTHING`, a.ID, a.Name, a.Department, roles, a.PasswordHash)
		if e != nil {
			return mapError(e)
		}
		if tag.RowsAffected() == 1 {
			if e = insertAudit(ctx, tx, g.Event{ID: uuid.NewString(), At: time.Now().UTC(), Actor: "local-demo-setup", Action: "provision_account", Object: a.ID, Department: a.Department, Revision: 1, Result: "success", Correlation: uuid.NewString(), AfterHash: g.Digest(a.Actor)}); e != nil {
				return e
			}
		}
	}
	return mapError(tx.Commit(ctx))
}
func (s *Store) Account(ctx context.Context, id string) (g.Account, error) {
	var a g.Account
	var roles []byte
	e := s.pool.QueryRow(ctx, `SELECT id,name,department,roles,password_hash,revision FROM agentregistry.governance_accounts WHERE id=$1`, id).Scan(&a.ID, &a.Name, &a.Department, &roles, &a.PasswordHash, &a.Revision)
	if e != nil {
		return a, mapError(e)
	}
	e = json.Unmarshal(roles, &a.Roles)
	return a, mapError(e)
}
func (s *Store) Accounts(ctx context.Context, actor g.Actor) ([]g.Actor, error) {
	if !actor.Has("admin") {
		return nil, g.ErrForbidden
	}
	out := []g.Actor{}
	rows, e := s.pool.Query(ctx, `SELECT id,name,department,roles,revision FROM agentregistry.governance_accounts WHERE department=$1 ORDER BY id`, actor.Department)
	if e != nil {
		return nil, mapError(e)
	}
	defer rows.Close()
	for rows.Next() {
		var a g.Actor
		var roles []byte
		if e = rows.Scan(&a.ID, &a.Name, &a.Department, &roles, &a.Revision); e != nil {
			return nil, mapError(e)
		}
		if e = json.Unmarshal(roles, &a.Roles); e != nil {
			return nil, mapError(e)
		}
		out = append(out, a)
	}
	return out, mapError(rows.Err())
}
func (s *Store) Assign(ctx context.Context, actor g.Actor, id string, roles []string, revision int) (g.Actor, error) {
	if !actor.Has("admin") || actor.ID == id {
		return g.Actor{}, g.ErrForbidden
	}
	var result g.Actor
	e := s.Within(ctx, actor, func(txUnit g.Unit) error {
		u := txUnit.(*unit)
		var a g.Actor
		var before []byte
		if e := u.tx.QueryRow(ctx, `SELECT id,name,department,roles,revision FROM agentregistry.governance_accounts WHERE id=$1 AND department=$2 FOR UPDATE`, id, actor.Department).Scan(&a.ID, &a.Name, &a.Department, &before, &a.Revision); e != nil {
			return mapError(e)
		}
		if a.Revision != revision {
			return g.ErrConflict
		}
		a.Roles = roles
		a.Revision++
		b, _ := json.Marshal(roles)
		if _, e := u.tx.Exec(ctx, `UPDATE agentregistry.governance_accounts SET roles=$1,revision=$2 WHERE id=$3`, b, a.Revision, id); e != nil {
			return mapError(e)
		}
		if e := u.Audit(ctx, g.Event{ID: uuid.NewString(), At: time.Now().UTC(), Actor: actor.ID, Action: "assign_roles", Object: id, Department: actor.Department, Revision: a.Revision, Result: "success", Correlation: uuid.NewString(), BeforeHash: g.Digest(json.RawMessage(before)), AfterHash: g.Digest(a)}); e != nil {
			return e
		}
		result = a
		return nil
	})
	return result, e
}
