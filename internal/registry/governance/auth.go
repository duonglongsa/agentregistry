package governance

import (
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/agentregistry-dev/agentregistry/pkg/registry/auth"
	"github.com/google/uuid"
	"golang.org/x/crypto/bcrypt"
)

const CookieName = "registry_governance_session"

type Account struct {
	Actor
	PasswordHash string `json:"passwordHash"`
}
type AuthConfig struct {
	Accounts   []Account `json:"accounts"`
	SigningKey string    `json:"signingKey"`
}
type attempt struct {
	Count int
	Until time.Time
}
type IdentityStore interface {
	Provision(context.Context, []Account) error
	Account(context.Context, string) (Account, error)
	Accounts(context.Context, Actor) ([]Actor, error)
	Assign(context.Context, Actor, string, []string, int) (Actor, error)
	Denied(context.Context, Event) error
}
type LocalAuth struct {
	identities IdentityStore
	accounts   map[string]Account
	key        []byte
	origins    map[string]bool
	mu         sync.Mutex
	attempts   map[string]attempt
	now        func() time.Time
}
type ticket struct {
	ID      string `json:"id"`
	Expires int64  `json:"exp"`
	Version string `json:"version"`
}

func LoadLocalAuth(address string) (*LocalAuth, error) {
	if os.Getenv("AGENT_REGISTRY_GOVERNANCE_LOCAL_DEMO") != "true" {
		return nil, errors.New("governance requires explicit local demo authentication; no public fallback")
	}
	host, _, e := net.SplitHostPort(address)
	ip := net.ParseIP(host)
	if e != nil || !(host == "localhost" || ip != nil && ip.IsLoopback()) {
		return nil, errors.New("local demo governance must bind to a loopback address")
	}
	path := os.Getenv("AGENT_REGISTRY_GOVERNANCE_AUTH_FILE")
	if path == "" {
		return nil, errors.New("governance auth file is required")
	}
	info, e := os.Stat(path)
	if e != nil {
		return nil, fmt.Errorf("read governance auth config: %w", e)
	}
	if info.Mode().Perm()&0077 != 0 {
		return nil, errors.New("governance auth file permissions must be 0600")
	}
	b, e := os.ReadFile(path)
	if e != nil {
		return nil, fmt.Errorf("read governance auth config: %w", e)
	}
	var cfg AuthConfig
	dec := json.NewDecoder(strings.NewReader(string(b)))
	dec.DisallowUnknownFields()
	if e = dec.Decode(&cfg); e != nil {
		return nil, fmt.Errorf("parse governance auth config: %w", e)
	}
	return NewLocalAuth(cfg, []string{"http://127.0.0.1:3000", "http://localhost:3000", "http://" + address})
}
func NewLocalAuth(cfg AuthConfig, origins []string) (*LocalAuth, error) {
	key, e := base64.RawURLEncoding.DecodeString(cfg.SigningKey)
	if e != nil || len(key) < 32 {
		return nil, errors.New("session signing key must contain at least 32 random bytes")
	}
	a := &LocalAuth{accounts: map[string]Account{}, key: key, origins: map[string]bool{}, attempts: map[string]attempt{}, now: time.Now}
	for _, u := range cfg.Accounts {
		if u.ID == "" || u.Name == "" || !namePattern.MatchString(u.Department) || len(u.Roles) == 0 {
			return nil, errors.New("invalid governance account")
		}
		if _, exists := a.accounts[u.ID]; exists {
			return nil, errors.New("duplicate governance account")
		}
		if cost, e := bcrypt.Cost([]byte(u.PasswordHash)); e != nil || cost < 12 {
			return nil, errors.New("account password must be bcrypt hashed with cost >=12")
		}
		for _, r := range u.Roles {
			if !oneOf(r, "owner", "tool_owner", "approver", "auditor", "admin") {
				return nil, errors.New("unknown governance role")
			}
		}
		a.accounts[u.ID] = u
	}
	if len(a.accounts) < 3 {
		return nil, errors.New("provision distinct owner, approver and auditor accounts")
	}
	for _, o := range origins {
		a.origins[o] = true
	}
	return a, nil
}
func (a *LocalAuth) Login(id, password, remote string) (string, error) {
	a.mu.Lock()
	v := a.attempts[remote]
	if a.now().After(v.Until) {
		v = attempt{Until: a.now().Add(5 * time.Minute)}
	}
	v.Count++
	a.attempts[remote] = v
	// Bound memory without allowing an attacker to evict an active source's limit.
	if len(a.attempts) > 10000 {
		for k, x := range a.attempts {
			if a.now().After(x.Until) {
				delete(a.attempts, k)
			}
		}
	}
	a.mu.Unlock()
	if v.Count > 20 {
		return "", auth.ErrUnauthenticated
	}
	u, ok := a.accounts[id]
	if a.identities != nil {
		var e error
		u, e = a.identities.Account(context.Background(), id)
		ok = e == nil
	}
	if !ok {
		bcrypt.CompareHashAndPassword([]byte("$2a$12$R9h/cIPz0gi.URNNX3kh2OPST9/PgBkqquzi.Ss7KIUgO2t0jWMUW"), []byte(password))
		return "", auth.ErrUnauthenticated
	}
	if bcrypt.CompareHashAndPassword([]byte(u.PasswordHash), []byte(password)) != nil {
		return "", auth.ErrUnauthenticated
	}
	payload, _ := json.Marshal(ticket{ID: id, Expires: a.now().Add(2 * time.Hour).Unix(), Version: Digest(u)})
	data := base64.RawURLEncoding.EncodeToString(payload)
	return data + "." + a.sign(data), nil
}
func (a *LocalAuth) sign(s string) string {
	h := hmac.New(sha256.New, a.key)
	h.Write([]byte(s))
	return base64.RawURLEncoding.EncodeToString(h.Sum(nil))
}
func (a *LocalAuth) Resolve(value string) (Actor, error) {
	parts := strings.Split(value, ".")
	if len(parts) != 2 || !hmac.Equal([]byte(a.sign(parts[0])), []byte(parts[1])) {
		return Actor{}, auth.ErrUnauthenticated
	}
	b, e := base64.RawURLEncoding.DecodeString(parts[0])
	if e != nil {
		return Actor{}, auth.ErrUnauthenticated
	}
	var t ticket
	if json.Unmarshal(b, &t) != nil || t.Expires <= a.now().Unix() {
		return Actor{}, auth.ErrUnauthenticated
	}
	u, ok := a.accounts[t.ID]
	if a.identities != nil {
		var e error
		u, e = a.identities.Account(context.Background(), t.ID)
		ok = e == nil
	}
	if !ok || t.Version != Digest(u) {
		return Actor{}, auth.ErrUnauthenticated
	}
	return u.Actor, nil
}
func SessionCookie(token string) *http.Cookie {
	max := 7200
	if token == "" {
		max = -1
	}
	return &http.Cookie{Name: CookieName, Value: token, Path: "/api/governance", HttpOnly: true, SameSite: http.SameSiteStrictMode, MaxAge: max, Secure: false}
}

// Guard disables all upstream API routes in governance mode, including anonymous
// compatibility, apply, batch, CLI HTTP and deployment paths. These paths lack
// department SQL visibility. UI assets and health remain available.
func (a *LocalAuth) Guard(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		p := r.URL.Path
		if p == "/v0/health" || p == "/health" {
			next.ServeHTTP(w, r)
			return
		}
		if !strings.HasPrefix(p, "/api/governance/") {
			if strings.HasPrefix(p, "/v0") || strings.HasPrefix(p, "/api/") || strings.HasPrefix(p, "/admin") || strings.HasPrefix(p, "/mcp") || strings.HasPrefix(p, "/plugin-marketplace") || p == "/metrics" || p == "/logging" {
				http.Error(w, "endpoint disabled in governance mode", http.StatusForbidden)
				return
			}
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		if r.Method != "GET" && r.Method != "HEAD" && !a.origins[r.Header.Get("Origin")] {
			http.Error(w, "same-origin request required", http.StatusForbidden)
			return
		}
		if p == "/api/governance/login" {
			next.ServeHTTP(w, r)
			return
		}
		cookie, e := r.Cookie(CookieName)
		if e != nil {
			http.Error(w, "sign in required", http.StatusUnauthorized)
			return
		}
		actor, e := a.Resolve(cookie.Value)
		if e != nil {
			http.Error(w, "session expired; sign in again", http.StatusUnauthorized)
			return
		}
		r = r.WithContext(auth.AuthSessionTo(r.Context(), actor))
		next.ServeHTTP(w, r)
	})
}
func RandomKey() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}

func (a *LocalAuth) Attach(ctx context.Context, store IdentityStore) error {
	accounts := make([]Account, 0, len(a.accounts))
	for _, v := range a.accounts {
		accounts = append(accounts, v)
	}
	if e := store.Provision(ctx, accounts); e != nil {
		return e
	}
	a.identities = store
	return nil
}
func (a *LocalAuth) Accounts(ctx context.Context) ([]Actor, error) {
	actor, e := Current(ctx)
	if e != nil {
		return nil, e
	}
	if a.identities == nil {
		return nil, ErrForbidden
	}
	return a.identities.Accounts(ctx, actor)
}
func (a *LocalAuth) Assign(ctx context.Context, id string, roles []string, revision int) (Actor, error) {
	result, e := a.assign(ctx, id, roles, revision)
	if e != nil && a.identities != nil {
		if actor, actorErr := Current(ctx); actorErr == nil {
			auditErr := a.identities.Denied(ctx, Event{ID: uuid.NewString(), At: a.now().UTC(), Actor: actor.ID, Action: "assign_roles", Object: id, Department: actor.Department, Result: "denied", Correlation: uuid.NewString()})
			if auditErr != nil {
				return Actor{}, fmt.Errorf("audit denied role assignment: %w", auditErr)
			}
		}
	}
	return result, e
}
func (a *LocalAuth) assign(ctx context.Context, id string, roles []string, revision int) (Actor, error) {
	actor, e := Current(ctx)
	if e != nil {
		return Actor{}, e
	}
	if !actor.Has("admin") || a.identities == nil {
		return Actor{}, ErrForbidden
	}
	if len(roles) == 0 {
		return Actor{}, ErrInvalid
	}
	seen := map[string]bool{}
	for _, r := range roles {
		if !oneOf(r, "owner", "tool_owner", "approver", "auditor", "admin") || seen[r] {
			return Actor{}, ErrInvalid
		}
		seen[r] = true
	}
	return a.identities.Assign(ctx, actor, id, roles, revision)
}

// RegistryBoundary denies access to the unscoped upstream store surface while
// governance uses its own scoped repository through the shared auth.Session.
type RegistryBoundary struct{}

func (RegistryBoundary) Check(context.Context, auth.Session, auth.PermissionAction, auth.Resource) error {
	return auth.ErrForbidden
}
func (RegistryBoundary) IsRegistryAdmin(context.Context, auth.Session) bool { return false }
