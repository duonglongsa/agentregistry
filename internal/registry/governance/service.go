package governance

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
)

type Service struct {
	repo Repository
	now  func() time.Time
}

func NewService(repo Repository) *Service {
	return &Service{repo: repo, now: func() time.Time { return time.Now().UTC() }}
}
func (s *Service) List(ctx context.Context, f Filter) (Page, error) {
	a, e := Current(ctx)
	if e != nil {
		return Page{}, e
	}
	return s.repo.List(ctx, a, f, s.now())
}
func (s *Service) Events(ctx context.Context, f Filter) (Events, error) {
	a, e := Current(ctx)
	if e != nil {
		return Events{}, e
	}
	return s.repo.Events(ctx, a, f)
}
func (s *Service) Get(ctx context.Context, id string) (Record, error) {
	a, e := Current(ctx)
	if e != nil {
		return Record{}, e
	}
	var r Record
	e = s.repo.Within(ctx, a, func(u Unit) error { var e error; r, e = u.Get(ctx, id); return e })
	return Effective(r, s.now()), e
}
func (s *Service) Execute(ctx context.Context, c Command, correlation string) (Record, error) {
	a, e := Current(ctx)
	if e != nil {
		return Record{}, e
	}
	c.Reason = strings.TrimSpace(c.Reason)
	now := s.now()
	var result Record
	if correlation == "" {
		correlation = uuid.NewString()
	}
	e = s.repo.Within(ctx, a, func(u Unit) error {
		var r Record
		var err error
		before := ""
		if c.Action == "create" {
			if !((c.Kind == "agent" && a.Has("owner")) || (c.Kind == "instance" && a.Has("owner")) || (c.Kind == "request" && a.Has("owner")) || (oneOf(c.Kind, "server", "tool") && (a.Has("tool_owner") || a.Has("admin")))) {
				return ErrForbidden
			}
			p := CleanPayload(c.Kind, c.Data)
			if p.Namespace == "" && oneOf(c.Kind, "agent", "server") {
				p.Namespace = a.Department
			}
			if p.Tag == "" && oneOf(c.Kind, "agent", "server") {
				p.Tag = "latest"
			}
			if oneOf(c.Kind, "agent", "server") && (p.Namespace != a.Department || !namePattern.MatchString(p.Tag)) {
				return ErrForbidden
			}
			if err := Validate(c.Kind, p, now); err != nil {
				return err
			}
			r = Record{ID: uuid.NewString(), Kind: c.Kind, Department: a.Department, Owner: a.ID, State: "draft", Version: 1, Parent: c.Parent, Tool: c.Tool, Data: p, CreatedAt: now, UpdatedAt: now}
			switch c.Kind {
			case "agent", "server":
				r.Parent = ""
				r.Tool = ""
				if c.Kind == "server" {
					r.State = "registered"
				}
				if err := u.Catalog(ctx, r); err != nil {
					return err
				}
			case "instance":
				parent, e := u.Get(ctx, c.Parent)
				if e != nil {
					return e
				}
				if parent.Data.RegistrationScope == "outside_mb" {
					return fmt.Errorf("%w: external agent recorded outside governance scope", ErrInvalid)
				}
				if parent.Kind != "agent" || parent.Owner != a.ID || oneOf(parent.State, "retired", "suspended") {
					return ErrForbidden
				}
				r.Tool = ""
				r.State = "registered"
			case "tool":
				parent, e := u.Get(ctx, c.Parent)
				if e != nil {
					return e
				}
				if parent.Kind != "server" || parent.State == "archived" || (parent.Owner != a.ID && !a.Has("admin")) {
					return ErrForbidden
				}
				r.Tool = ""
			case "request":
				if err := s.requestContext(ctx, u, a, r, now); err != nil {
					return err
				}
				if err := s.bindRequest(ctx, u, &r); err != nil {
					return err
				}
			}
		} else {
			r, err = u.Get(ctx, c.ID)
			if err != nil {
				return err
			}
			if r.Kind != c.Kind {
				return ErrInvalid
			}
			if r.Version != c.BaseVersion {
				return ErrConflict
			}
			before = Digest(r)
			switch c.Action {
			case "update":
				if r.Owner != a.ID || !((oneOf(r.Kind, "agent", "request") && a.Has("owner")) || (r.Kind == "tool" && (a.Has("tool_owner") || a.Has("admin")))) {
					return ErrForbidden
				}
				if r.State != "draft" {
					return ErrConflict
				}
				p := CleanPayload(r.Kind, c.Data)
				p.Name = r.Data.Name
				p.Namespace = r.Data.Namespace
				p.Tag = r.Data.Tag
				if r.Kind != "request" {
					p.Snapshot = r.Data.Snapshot
				}
				p.RequestType = r.Data.RequestType
				if err := Validate(r.Kind, p, now); err != nil {
					return err
				}
				if r.Kind == "request" {
					candidate := r
					candidate.Data = p
					if e := s.requestContext(ctx, u, a, candidate, now); e != nil {
						return e
					}
				}
				r.Data = p
				if r.Kind == "request" {
					if e := s.bindRequest(ctx, u, &r); e != nil {
						return e
					}
				}
			case "submit":
				if r.Kind != "request" || r.Owner != a.ID || !a.Has("owner") {
					return ErrForbidden
				}
				if r.State != "draft" {
					return ErrConflict
				}
				if err := Validate("request", r.Data, now); err != nil {
					return err
				}
				if err := s.requestContext(ctx, u, a, r, now); err != nil {
					return err
				}
				r.State = "pending_review"
				if r.Data.RequestType == "agent" {
					p, e := u.Get(ctx, r.Parent)
					if e != nil {
						return e
					}
					if p.State != "draft" {
						return ErrConflict
					}
					p.State = "pending_review"
					p.Version++
					p.UpdatedAt = now
					if e = u.Save(ctx, p, false); e != nil {
						return e
					}
				}
			case "approve", "reject":
				if !a.Has("approver") {
					return ErrForbidden
				}
				if r.Owner == a.ID {
					return ErrForbidden
				}
				if !oneOf(r.Kind, "request", "tool") {
					return ErrInvalid
				}
				if !(r.Kind == "request" && r.State == "pending_review" || r.Kind == "tool" && r.State == "draft") {
					return ErrConflict
				}
				if c.Action == "reject" && c.Reason == "" {
					return fmt.Errorf("%w: rejection reason required", ErrInvalid)
				}
				if r.Kind == "request" {
					if err := s.requestContext(ctx, u, Actor{ID: r.Owner, Department: r.Department}, r, now); err != nil {
						return err
					}
					if r.Hash != Digest(r.Data) {
						return ErrConflict
					}
					if r.Data.RequestType == "agent" {
						p, e := u.Get(ctx, r.Parent)
						if e != nil {
							return e
						}
						if p.State != "pending_review" {
							return ErrConflict
						}
						p.State = "draft"
						if c.Action == "approve" {
							p.State = "approved"
						}
						p.Version++
						p.UpdatedAt = now
						if e = u.Save(ctx, p, false); e != nil {
							return e
						}
					} else if c.Action == "approve" {
						grant := Record{ID: uuid.NewString(), Kind: "grant", Department: r.Department, Owner: r.Owner, State: "approved", Version: 1, Parent: r.Parent, Tool: r.Tool, Source: r.ID, Data: r.Data, Hash: r.Hash, DecisionBy: a.ID, DecidedAt: &now, CreatedAt: now, UpdatedAt: now}
						if e := u.Save(ctx, grant, true); e != nil {
							return e
						}
					}
				}
				r.State = "rejected"
				if c.Action == "approve" {
					r.State = "approved"
				}
				r.DecisionBy = a.ID
				r.DecidedAt = &now
				r.Reason = c.Reason
			case "cancel":
				if r.Kind != "request" || r.Owner != a.ID || !a.Has("owner") {
					return ErrForbidden
				}
				if !oneOf(r.State, "draft", "pending_review") {
					return ErrConflict
				}
				if r.Data.RequestType == "agent" && r.State == "pending_review" {
					p, e := u.Get(ctx, r.Parent)
					if e != nil {
						return e
					}
					if p.State != "pending_review" {
						return ErrConflict
					}
					p.State = "draft"
					p.Version++
					p.UpdatedAt = now
					if e = u.Save(ctx, p, false); e != nil {
						return e
					}
				}
				r.State = "cancelled"
			case "revoke":
				if r.Kind != "grant" || !(a.Has("approver") || a.Has("admin")) {
					return ErrForbidden
				}
				if Effective(r, now).State != "approved" {
					return ErrConflict
				}
				if c.Reason == "" {
					return fmt.Errorf("%w: revocation reason required", ErrInvalid)
				}
				r.State = "revoked"
				r.Reason = c.Reason
				r.RevokedAt = &now
				r.RevokedBy = a.ID
			case "suspend", "resume", "archive":
				if !(a.Has("approver") || a.Has("admin")) {
					return ErrForbidden
				}
				if c.Action == "suspend" {
					if r.Kind != "agent" || r.State != "approved" {
						return ErrConflict
					}
					if c.Reason == "" {
						return fmt.Errorf("%w: suspension reason required", ErrInvalid)
					}
					r.State = "suspended"
				}
				if c.Action == "resume" {
					if r.Kind != "agent" || r.State != "suspended" {
						return ErrConflict
					}
					r.State = "approved"
				}
				if c.Action == "archive" {
					if !oneOf(r.Kind, "agent", "server", "tool") || oneOf(r.State, "retired", "archived") {
						return ErrConflict
					}
					if c.Reason == "" {
						return fmt.Errorf("%w: archive reason required", ErrInvalid)
					}
					r.State = "archived"
					if r.Kind == "agent" {
						r.State = "retired"
					}
				}
				r.Reason = c.Reason
			default:
				return ErrInvalid
			}
			r.Version++
			r.UpdatedAt = now
		}
		r.Hash = Digest(r.Data)
		if err := u.Save(ctx, r, c.Action == "create"); err != nil {
			return err
		}
		event := Event{ID: uuid.NewString(), At: now, Actor: a.ID, Action: c.Action, Object: r.ID, Parent: r.Parent, Department: a.Department, Revision: r.Version, Reason: c.Reason, Result: "success", Correlation: correlation, BeforeHash: before, AfterHash: Digest(r)}
		if err := u.Audit(ctx, event); err != nil {
			return err
		}
		result = Effective(r, now)
		return nil
	})
	if e != nil && (errors.Is(e, ErrForbidden) || errors.Is(e, ErrConflict) || errors.Is(e, ErrNotFound)) {
		auditErr := s.repo.Denied(ctx, Event{ID: uuid.NewString(), At: now, Actor: a.ID, Action: c.Action, Object: c.ID, Department: a.Department, Result: "denied", Correlation: correlation})
		if auditErr != nil {
			return Record{}, fmt.Errorf("persist denied audit: %w", auditErr)
		}
	}
	return result, e
}
func (s *Service) requestContext(ctx context.Context, u Unit, a Actor, r Record, now time.Time) error {
	p, e := u.Get(ctx, r.Parent)
	if e != nil {
		return e
	}
	if p.Owner != a.ID {
		return ErrForbidden
	}
	if r.Data.RequestType == "agent" {
		if p.Data.RegistrationScope == "outside_mb" {
			return fmt.Errorf("%w: external agent recorded outside governance scope", ErrInvalid)
		}
		if r.Data.TargetHash != "" && r.Data.TargetHash != p.Hash {
			return ErrConflict
		}
		if p.Kind != "agent" || !oneOf(p.State, "draft", "pending_review") {
			return ErrConflict
		}
		return nil
	}
	if p.Kind != "instance" {
		return ErrInvalid
	}
	agent, e := u.Get(ctx, p.Parent)
	if e != nil {
		return e
	}
	if agent.State != "approved" {
		return ErrConflict
	}
	tool, e := u.Get(ctx, r.Tool)
	if e != nil {
		return e
	}
	if r.Data.TargetHash != "" && r.Data.TargetHash != tool.Hash {
		return ErrConflict
	}
	if tool.Kind != "tool" || tool.State != "approved" {
		return ErrConflict
	}
	server, e := u.Get(ctx, tool.Parent)
	if e != nil {
		return e
	}
	if server.State != "registered" {
		return ErrConflict
	}
	if r.Data.ExpiresAt == nil || !r.Data.ExpiresAt.After(now) {
		return ErrConflict
	}
	return nil
}

// bindRequest records immutable display references on the draft. API input cannot
// set these fields. The reviewed tool/profile remains linked by ID and digest.
func (s *Service) bindRequest(ctx context.Context, u Unit, r *Record) error {
	p, e := u.Get(ctx, r.Parent)
	if e != nil {
		return e
	}
	if r.Data.RequestType == "agent" {
		r.Data.Name = p.Data.Name
		r.Data.Snapshot = p.Data.Tag
		r.Data.TargetHash = p.Hash
		return nil
	}
	tool, e := u.Get(ctx, r.Tool)
	if e != nil {
		return e
	}
	server, e := u.Get(ctx, tool.Parent)
	if e != nil {
		return e
	}
	r.Data.Name = tool.Data.Name
	r.Data.Snapshot = tool.Data.Snapshot
	r.Data.TargetHash = tool.Hash
	r.Data.Environment = p.Data.Environment
	r.Data.ServerReference = server.Data.Namespace + "/" + server.Data.Name + "@" + server.Data.Tag
	return nil
}
