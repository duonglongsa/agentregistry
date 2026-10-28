#!/usr/bin/env python3
"""Opt-in local seed and HTTP acceptance. Never contacts a runtime or MCP server."""
import argparse
import datetime
import http.cookiejar
import json
import pathlib
import urllib.error
import urllib.parse
import urllib.request

ROOT = pathlib.Path(__file__).resolve().parents[2]

class Client:
    def __init__(self, base, identity, password):
        self.base = base
        self.opener = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(http.cookiejar.CookieJar()))
        self.call("POST", "/login", {"id": identity, "password": password})
        self.actor = self.call("GET", "/me")
        if self.call("GET", "/mode")["mode"] != "local_governance_demo":
            raise RuntimeError("server is not in explicit local demo mode")
        if not self.actor["id"].startswith("demo-"):
            raise RuntimeError("refusing seed for an account not provisioned as demo")

    def call(self, method, path, data=None, expected=200):
        body = None if data is None else json.dumps(data).encode()
        req = urllib.request.Request(self.base + "/api/governance" + path, data=body, method=method,
                                     headers={"Content-Type": "application/json", "Origin": "http://127.0.0.1:3000"})
        try:
            with self.opener.open(req) as response:
                if response.status != expected:
                    raise RuntimeError(f"unexpected HTTP {response.status}")
                return json.load(response)
        except urllib.error.HTTPError as error:
            if error.code == expected:
                return None
            raise RuntimeError(f"{method} {path}: HTTP {error.code}: {error.read().decode()}") from error

    def records(self, kind=""):
        records, offset = [], 0
        while True:
            page = self.call("GET", "/records?" + urllib.parse.urlencode({"kind": kind, "offset": offset, "limit": 100}))
            records.extend(page["items"])
            offset += len(page["items"])
            if offset >= page["total"]:
                return records

    def create(self, kind, data, parent="", tool=""):
        return self.call("POST", "/commands", {"action": "create", "kind": kind, "baseVersion": 0, "parent": parent, "tool": tool, "data": data})

    def action(self, record, action, reason="", expected=200):
        return self.call("POST", "/commands", {"action": action, "kind": record["kind"], "id": record["id"],
                    "baseVersion": record["version"], "reason": reason, "data": {"name": ""}}, expected)


def main():
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("--local-demo", action="store_true", required=True)
    parser.add_argument("--base", default="http://127.0.0.1:12121")
    parser.add_argument("--workflow", action="store_true", help="also approve/revoke and suspend/resume for acceptance")
    args = parser.parse_args()
    target = urllib.parse.urlparse(args.base)
    if target.hostname not in ("127.0.0.1", "localhost") or target.scheme != "http" or target.port != 12121:
        raise RuntimeError("seed accepts only the local demo backend on port 12121")
    credentials = json.loads((ROOT / ".local/governance/credentials.json").read_text())
    clients = {identity: Client(args.base, identity, credentials[identity]) for identity in
               ("demo-long", "demo-tool-owner", "demo-approver", "demo-auditor", "demo-ea")}
    owner, tools, approver, auditor, ea = [clients[key] for key in clients]
    existing = owner.records()
    if any(not row["owner"].startswith("demo-") for row in existing):
        raise RuntimeError("refusing to seed a dataset containing non-demo ownership")
    def ensure(client, kind, name, data, parent=""):
        matches = [r for r in client.records(kind) if r["data"]["name"] == name and r.get("parent", "") == parent]
        if matches:
            return matches[0]
        return client.create(kind, {"name": name, **data}, parent)
    agent = ensure(owner, "agent", "ktv-agent", {"description": "KTUD demo assistant for Jira issue preparation", "purpose": "Track demo engineering work", "risk": "medium"})
    instance = ensure(owner, "instance", "dev", {"environment": "dev", "location": "workstation"}, agent["id"])
    server = ensure(tools, "server", "jira-mcp", {"tag": "demo-v1", "description": "Metadata catalog only; no MCP server is contacted"})
    for name, risk in (("create_issue", "medium"), ("delete_issue", "high")):
        tool = ensure(tools, "tool", name, {"description": f"Demo {name}; metadata only", "snapshot": "v1", "classification": "write", "risk": risk,
            "inputSchema": {"type": "object", "properties": {"project": {"type": "string"}}, "required": ["project"], "additionalProperties": False}}, server["id"])
        if tool["state"] == "draft":
            approver.action(tool, "approve")
    if args.workflow:
        if agent["state"] == "draft":
            review = next((r for r in owner.records("request") if r.get("parent") == agent["id"] and r["state"] in ("draft", "pending_review")), None)
            review = review or owner.create("request", {"name": "", "requestType": "agent", "purpose": "Review demo profile"}, agent["id"])
            if review["state"] == "draft":
                review = owner.action(review, "submit")
            owner.action(review, "approve", expected=403)
            approver.action(review, "approve")
        if not owner.records("grant"):
            tool = next(t for t in owner.records("tool") if t["data"]["name"] == "create_issue")
            expiry = (datetime.datetime.now(datetime.timezone.utc) + datetime.timedelta(days=7)).isoformat()
            request = owner.create("request", {"name": "", "requestType": "access", "purpose": "Create demo project issues", "operations": ["create_issue"], "constraints": {"project": "DEMO"}, "expiresAt": expiry}, instance["id"], tool["id"])
            request = owner.action(request, "submit")
            owner.action(request, "approve", expected=403)
            auditor.action(request, "approve", expected=403)
            ea.action(request, "approve", expected=404)
            approver.action(request, "approve")
        grants = owner.records("grant")
        assert len(grants) == 1, "exactly create_issue must have a grant"
        assert next(t for t in owner.records("tool") if t["id"] == grants[0]["tool"])["data"]["name"] == "create_issue"
        if grants[0]["state"] == "approved":
            approver.action(grants[0], "revoke", "Demo acceptance: revoke Registry approval")
        agent = owner.call("GET", "/records/" + agent["id"])
        if agent["state"] == "approved":
            agent = approver.action(agent, "suspend", "Demo acceptance review")
            approver.action(agent, "resume")
        assert owner.records("grant")[0]["state"] == "revoked"
        assert ea.records() == []
        assert auditor.call("GET", "/audit")["total"] > 0
    print("Demo seed verified. Existing records preserved; no runtime calls. Registry decisions only.")

if __name__ == "__main__":
    main()
