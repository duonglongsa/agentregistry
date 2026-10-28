// Package governance records Registry decisions only. It never calls a runtime.
package governance

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
)

var (
	ErrForbidden = errors.New("outside your permissions or scope")
	ErrConflict  = errors.New("record changed or transition is not allowed; refresh and retry")
	ErrInvalid   = errors.New("invalid governance input")
	ErrNotFound  = errors.New("record not found")
)

const Notice = "Quyền được ghi nhận tại Registry; chưa tích hợp kiểm soát thực thi"

type Actor struct {
	Revision   int      `json:"revision"`
	ID         string   `json:"id"`
	Name       string   `json:"name"`
	Department string   `json:"department"`
	Roles      []string `json:"roles"`
}

func (a Actor) Has(role string) bool {
	for _, r := range a.Roles {
		if r == role {
			return true
		}
	}
	return false
}
func (a Actor) Principal() auth.Principal { return auth.Principal{} }
func Current(ctx context.Context) (Actor, error) {
	s, ok := auth.AuthSessionFrom(ctx)
	if !ok {
		return Actor{}, auth.ErrUnauthenticated
	}
	a, ok := s.(Actor)
	if !ok || a.ID == "" {
		return Actor{}, auth.ErrUnauthenticated
	}
	return a, nil
}

type Identity struct {
	Issuer          string `json:"issuer,omitempty" maxLength:"512"`
	Subject         string `json:"subject,omitempty" maxLength:"255"`
	ClientReference string `json:"clientReference,omitempty" maxLength:"255"`
}

// Payload contains metadata only. Submitted requests and reviewed tools freeze this data.
type Payload struct {
	RegistrationScope string            `json:"registrationScope,omitempty" enum:"internal,outside_mb,"`
	TargetHash        string            `json:"targetHash,omitempty"`
	ServerReference   string            `json:"serverReference,omitempty"`
	Name              string            `json:"name" maxLength:"255"`
	Namespace         string            `json:"namespace,omitempty" maxLength:"255"`
	Tag               string            `json:"tag,omitempty" maxLength:"255"`
	Description       string            `json:"description,omitempty" maxLength:"4000"`
	Purpose           string            `json:"purpose,omitempty" maxLength:"2000"`
	Risk              string            `json:"risk,omitempty" enum:"low,medium,high,"`
	Environment       string            `json:"environment,omitempty"`
	Location          string            `json:"location,omitempty"`
	Endpoint          string            `json:"endpoint,omitempty" maxLength:"2048"`
	Identity          Identity          `json:"identity,omitempty"`
	Classification    string            `json:"classification,omitempty"`
	InputSchema       json.RawMessage   `json:"inputSchema,omitempty"`
	Snapshot          string            `json:"snapshot,omitempty" maxLength:"255"`
	Operations        []string          `json:"operations,omitempty" maxItems:"20"`
	Constraints       map[string]string `json:"constraints,omitempty"`
	ExpiresAt         *time.Time        `json:"expiresAt,omitempty"`
	RequestType       string            `json:"requestType,omitempty"`
}
type Record struct {
	ID         string     `json:"id"`
	Kind       string     `json:"kind"`
	Department string     `json:"department"`
	Owner      string     `json:"owner"`
	State      string     `json:"state"`
	Version    int        `json:"version"`
	Parent     string     `json:"parent,omitempty"`
	Tool       string     `json:"tool,omitempty"`
	Source     string     `json:"source,omitempty"`
	Data       Payload    `json:"data"`
	Hash       string     `json:"hash"`
	DecisionBy string     `json:"decisionBy,omitempty"`
	DecidedAt  *time.Time `json:"decidedAt,omitempty"`
	RevokedAt  *time.Time `json:"revokedAt,omitempty"`
	RevokedBy  string     `json:"revokedBy,omitempty"`
	Reason     string     `json:"reason,omitempty"`
	CreatedAt  time.Time  `json:"createdAt"`
	UpdatedAt  time.Time  `json:"updatedAt"`
}
type Command struct {
	Action      string  `json:"action" enum:"create,update,submit,approve,reject,cancel,revoke,suspend,resume,archive"`
	Kind        string  `json:"kind" enum:"agent,instance,server,tool,request,grant"`
	ID          string  `json:"id,omitempty"`
	BaseVersion int     `json:"baseVersion" minimum:"0"`
	Parent      string  `json:"parent,omitempty"`
	Tool        string  `json:"tool,omitempty"`
	Reason      string  `json:"reason,omitempty" maxLength:"2000"`
	Data        Payload `json:"data"`
}
type Filter struct {
	Kind   string `query:"kind" json:"kind"`
	Parent string `query:"parent" json:"parent"`
	Actor  string `query:"actor" json:"actor"`
	Action string `query:"action" json:"action"`
	State  string `query:"state" json:"state"`
	Search string `query:"search" json:"search" maxLength:"255"`
	From   string `query:"from" json:"from"`
	Until  string `query:"until" json:"until"`
	Offset int    `query:"offset" json:"offset" minimum:"0" default:"0"`
	Limit  int    `query:"limit" json:"limit" minimum:"1" maximum:"100" default:"25"`
}
type Page struct {
	Items []Record `json:"items"`
	Total int      `json:"total"`
}
type Event struct {
	ID          string    `json:"id"`
	At          time.Time `json:"at"`
	Actor       string    `json:"actor"`
	Action      string    `json:"action"`
	Object      string    `json:"object"`
	Parent      string    `json:"parent"`
	Department  string    `json:"department"`
	Revision    int       `json:"revision"`
	Reason      string    `json:"reason"`
	Result      string    `json:"result"`
	Correlation string    `json:"correlation"`
	BeforeHash  string    `json:"beforeHash"`
	AfterHash   string    `json:"afterHash"`
}
type Events struct {
	Items []Event `json:"items"`
	Total int     `json:"total"`
}
type Unit interface {
	Get(context.Context, string) (Record, error)
	Save(context.Context, Record, bool) error
	Catalog(context.Context, Record) error
	Audit(context.Context, Event) error
}
type Repository interface {
	Within(context.Context, Actor, func(Unit) error) error
	List(context.Context, Actor, Filter, time.Time) (Page, error)
	Events(context.Context, Actor, Filter) (Events, error)
	Denied(context.Context, Event) error
}

func Digest(v any) string {
	b, _ := json.Marshal(v)
	h := sha256.Sum256(b)
	return hex.EncodeToString(h[:])
}
func Effective(r Record, now time.Time) Record {
	if r.Kind == "grant" && r.State == "approved" && r.Data.ExpiresAt != nil && !r.Data.ExpiresAt.After(now) {
		r.State = "expired"
	}
	return r
}
