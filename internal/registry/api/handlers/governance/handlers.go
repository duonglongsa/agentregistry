// Package governance is the Huma transport for Registry governance.
package governance

import (
	"context"
	"encoding/csv"
	"errors"
	"net/http"
	"strconv"
	"strings"
	"time"

	g "github.com/agentregistry-dev/agentregistry/internal/registry/governance"
	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/danielgtaylor/huma/v2"
)

func apiError(e error) error {
	switch {
	case e == nil:
		return nil
	case errors.Is(e, auth.ErrUnauthenticated):
		return huma.Error401Unauthorized("sign in required")
	case errors.Is(e, g.ErrForbidden):
		return huma.Error403Forbidden(e.Error())
	case errors.Is(e, g.ErrNotFound):
		return huma.Error404NotFound("record not found")
	case errors.Is(e, g.ErrConflict):
		return huma.Error409Conflict(e.Error())
	case errors.Is(e, g.ErrInvalid):
		return huma.Error422UnprocessableEntity(e.Error())
	default:
		return huma.Error500InternalServerError("unable to persist governance operation")
	}
}

type RecordsOutput struct{ Body g.Page }
type RecordOutput struct{ Body g.Record }
type EventsOutput struct{ Body g.Events }
type CommandInput struct{ Body g.Command }
type LoginInput struct {
	Body struct {
		ID       string `json:"id" minLength:"1" maxLength:"255"`
		Password string `json:"password" minLength:"1" maxLength:"72"`
	}
}
type LoginOutput struct {
	Cookie http.Cookie `header:"Set-Cookie"`
	Body   struct {
		Status string `json:"status"`
	}
}
type ActorOutput struct{ Body g.Actor }

func Register(api huma.API, s *g.Service, a *g.LocalAuth) {
	base := "/api/governance"
	huma.Get(api, base+"/mode", func(ctx context.Context, _ *struct{}) (*struct {
		Body struct {
			Mode   string `json:"mode"`
			Notice string `json:"notice"`
		}
	}, error) {
		out := &struct {
			Body struct {
				Mode   string `json:"mode"`
				Notice string `json:"notice"`
			}
		}{}
		out.Body.Mode = "local_governance_demo"
		out.Body.Notice = g.Notice
		return out, nil
	})
	huma.Get(api, base+"/accounts", func(ctx context.Context, _ *struct{}) (*struct{ Body []g.Actor }, error) {
		actors, e := a.Accounts(ctx)
		return &struct{ Body []g.Actor }{actors}, apiError(e)
	})
	huma.Post(api, base+"/accounts/{id}/roles", func(ctx context.Context, in *struct {
		ID   string `path:"id"`
		Body struct {
			Roles       []string `json:"roles" minItems:"1" maxItems:"5"`
			BaseVersion int      `json:"baseVersion" minimum:"1"`
		}
	}) (*ActorOutput, error) {
		actor, e := a.Assign(ctx, in.ID, in.Body.Roles, in.Body.BaseVersion)
		return &ActorOutput{Body: actor}, apiError(e)
	})
	huma.Post(api, base+"/login", func(ctx context.Context, in *LoginInput) (*LoginOutput, error) {
		if a == nil {
			return nil, huma.Error503ServiceUnavailable("governance authentication unavailable")
		}
		// A global limiter also bounds attacks using arbitrary login IDs or headers.
		token, e := a.Login(in.Body.ID, in.Body.Password, "local-demo")
		if e != nil {
			return nil, apiError(e)
		}
		out := &LoginOutput{Cookie: *g.SessionCookie(token)}
		out.Body.Status = "ok"
		return out, nil
	})
	huma.Post(api, base+"/logout", func(ctx context.Context, _ *struct{}) (*LoginOutput, error) {
		out := &LoginOutput{Cookie: *g.SessionCookie("")}
		out.Body.Status = "ok"
		return out, nil
	})
	huma.Get(api, base+"/me", func(ctx context.Context, _ *struct{}) (*ActorOutput, error) {
		actor, e := g.Current(ctx)
		return &ActorOutput{Body: actor}, apiError(e)
	})
	huma.Get(api, base+"/records", func(ctx context.Context, in *g.Filter) (*RecordsOutput, error) {
		p, e := s.List(ctx, *in)
		return &RecordsOutput{Body: p}, apiError(e)
	})
	huma.Get(api, base+"/records/{id}", func(ctx context.Context, in *struct {
		ID string `path:"id"`
	}) (*RecordOutput, error) {
		r, e := s.Get(ctx, in.ID)
		return &RecordOutput{Body: r}, apiError(e)
	})
	huma.Post(api, base+"/commands", func(ctx context.Context, in *CommandInput) (*RecordOutput, error) {
		r, e := s.Execute(ctx, in.Body, "")
		return &RecordOutput{Body: r}, apiError(e)
	})
	huma.Get(api, base+"/audit", func(ctx context.Context, in *g.Filter) (*EventsOutput, error) {
		if _, e := time.Parse(time.RFC3339, in.From); in.From != "" && e != nil {
			return nil, huma.Error422UnprocessableEntity("from must be RFC3339")
		}
		if _, e := time.Parse(time.RFC3339, in.Until); in.Until != "" && e != nil {
			return nil, huma.Error422UnprocessableEntity("until must be RFC3339")
		}
		p, e := s.Events(ctx, *in)
		return &EventsOutput{Body: p}, apiError(e)
	})
	huma.Get(api, base+"/audit.csv", func(ctx context.Context, in *g.Filter) (*struct {
		ContentType string `header:"Content-Type"`
		Disposition string `header:"Content-Disposition"`
		Body        []byte
	}, error) {
		// Iterate scoped, paginated SQL; never export an unscoped list. A repeatable
		// snapshot is provided per page; immutable rows remain stable after insertion.
		var b strings.Builder
		w := csv.NewWriter(&b)
		w.Write([]string{"timestamp_utc", "actor", "action", "object", "department", "revision", "reason", "result", "correlation"})
		in.Offset = 0
		in.Limit = 100
		if in.Until == "" {
			in.Until = time.Now().UTC().Format(time.RFC3339Nano)
		}
		for _, v := range []string{in.From, in.Until} {
			if v != "" {
				if _, e := time.Parse(time.RFC3339Nano, v); e != nil {
					return nil, huma.Error422UnprocessableEntity("time filters must be RFC3339")
				}
			}
		}
		for {
			p, e := s.Events(ctx, *in)
			if e != nil {
				return nil, apiError(e)
			}
			for _, v := range p.Items {
				w.Write([]string{v.At.UTC().Format(time.RFC3339Nano), safeCSV(v.Actor), safeCSV(v.Action), safeCSV(v.Object), safeCSV(v.Department), safeCSV(strconv.Itoa(v.Revision)), safeCSV(v.Reason), safeCSV(v.Result), safeCSV(v.Correlation)})
			}
			in.Offset += len(p.Items)
			if in.Offset >= p.Total || len(p.Items) == 0 {
				break
			}
			if b.Len() > 10*1024*1024 {
				return nil, huma.NewError(http.StatusRequestEntityTooLarge, "narrow the export date range")
			}
		}
		w.Flush()
		return &struct {
			ContentType string `header:"Content-Type"`
			Disposition string `header:"Content-Disposition"`
			Body        []byte
		}{"text/csv; charset=utf-8", "attachment; filename=governance-audit.csv", []byte(b.String())}, nil
	})
}
func safeCSV(v string) string {
	trim := strings.TrimLeft(v, " \t\r\n")
	if len(trim) > 0 && strings.ContainsAny(trim[:1], "=+-@") || strings.HasPrefix(v, "\t") || strings.HasPrefix(v, "\r") || strings.HasPrefix(v, "\n") {
		return "'" + v
	}
	return v
}
