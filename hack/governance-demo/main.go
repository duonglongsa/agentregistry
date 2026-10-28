// Provision local-only demo accounts. Credentials are generated, never checked in.
package main

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	g "github.com/agentregistry-dev/agentregistry/internal/registry/governance"
	"golang.org/x/crypto/bcrypt"
)

func main() {
	dir := ".local/governance"
	if len(os.Args) > 1 {
		dir = os.Args[1]
	}
	if e := os.MkdirAll(dir, 0700); e != nil {
		panic(e)
	}
	path := filepath.Join(dir, "auth.json")
	if _, e := os.Stat(path); e == nil {
		fmt.Println("Existing demo accounts preserved:", path)
		return
	}
	cfg := g.AuthConfig{SigningKey: g.RandomKey()}
	passwords := map[string]string{}
	users := []g.Actor{
		{ID: "demo-long", Name: "Long (demo)", Department: "ktud", Roles: []string{"owner"}},
		{ID: "demo-tool-owner", Name: "Tool Owner (demo)", Department: "ktud", Roles: []string{"tool_owner"}},
		{ID: "demo-approver", Name: "Approver (demo)", Department: "ktud", Roles: []string{"approver"}},
		{ID: "demo-auditor", Name: "Auditor (demo)", Department: "ktud", Roles: []string{"auditor"}},
		{ID: "demo-admin", Name: "Admin (demo)", Department: "ktud", Roles: []string{"admin", "tool_owner", "approver"}},
		{ID: "demo-ea", Name: "EA Owner (demo)", Department: "ea", Roles: []string{"owner", "approver"}},
	}
	for _, a := range users {
		pass := g.RandomKey()[:24]
		hash, e := bcrypt.GenerateFromPassword([]byte(pass), 12)
		if e != nil {
			panic(e)
		}
		cfg.Accounts = append(cfg.Accounts, g.Account{Actor: a, PasswordHash: string(hash)})
		passwords[a.ID] = pass
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	if e := os.WriteFile(path, b, 0600); e != nil {
		panic(e)
	}
	b, _ = json.MarshalIndent(passwords, "", "  ")
	if e := os.WriteFile(filepath.Join(dir, "credentials.json"), b, 0600); e != nil {
		panic(e)
	}
	fmt.Println("Local demo accounts created. Passwords:", filepath.Join(dir, "credentials.json"))
}
