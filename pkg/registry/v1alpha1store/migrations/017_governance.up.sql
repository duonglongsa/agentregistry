-- Registry-only governance; no runtime state is represented here.
CREATE UNIQUE INDEX agents_governance_uid ON agents(uid);
CREATE UNIQUE INDEX mcp_servers_governance_uid ON mcp_servers(uid);
CREATE TABLE governance_records (
 id uuid PRIMARY KEY,
 kind text NOT NULL CHECK (kind IN ('agent','server','instance','tool','request','grant')),
 department text NOT NULL,
 owner_id text NOT NULL,
 state text NOT NULL,
 version integer NOT NULL CHECK(version > 0),
 parent_id uuid REFERENCES governance_records(id) ON DELETE RESTRICT,
 tool_id uuid REFERENCES governance_records(id) ON DELETE RESTRICT,
 source_id uuid UNIQUE REFERENCES governance_records(id) ON DELETE RESTRICT,
 agent_uid uuid REFERENCES agents(uid) ON DELETE RESTRICT,
 server_uid uuid REFERENCES mcp_servers(uid) ON DELETE RESTRICT,
 data jsonb NOT NULL CHECK(jsonb_typeof(data)='object'),
 hash text NOT NULL,
 decision_by text NOT NULL DEFAULT '',
 decided_at timestamptz,
 revoked_at timestamptz,
 revoked_by text NOT NULL DEFAULT '',
 reason text NOT NULL DEFAULT '',
 created_at timestamptz NOT NULL DEFAULT now(),
 updated_at timestamptz NOT NULL DEFAULT now(),
 UNIQUE(id,department),
 FOREIGN KEY(parent_id,department) REFERENCES governance_records(id,department) ON DELETE RESTRICT,
 FOREIGN KEY(tool_id,department) REFERENCES governance_records(id,department) ON DELETE RESTRICT,
 FOREIGN KEY(source_id,department) REFERENCES governance_records(id,department) ON DELETE RESTRICT,
 CHECK ((kind='agent') = (agent_uid IS NOT NULL)),
 CHECK ((kind='server') = (server_uid IS NOT NULL)),
 CHECK ((kind IN ('instance','tool','request','grant')) = (parent_id IS NOT NULL)),
 CHECK ((kind='grant') = (source_id IS NOT NULL)),
 CHECK (kind<>'grant' OR state IN ('approved','revoked')),
 CHECK (kind<>'agent' OR state IN ('draft','pending_review','approved','suspended','retired')),
 CHECK (kind<>'request' OR state IN ('draft','pending_review','approved','rejected','cancelled')),
 CHECK (kind<>'tool' OR state IN ('draft','approved','rejected','archived'))
);
CREATE UNIQUE INDEX governance_agent_ref ON governance_records(agent_uid) WHERE agent_uid IS NOT NULL;
CREATE UNIQUE INDEX governance_server_ref ON governance_records(server_uid) WHERE server_uid IS NOT NULL;
CREATE UNIQUE INDEX governance_instance_identity ON governance_records(parent_id,(data->>'environment'),(data->>'name')) WHERE kind='instance';
CREATE UNIQUE INDEX governance_tool_identity ON governance_records(parent_id,(data->>'snapshot'),(data->>'name')) WHERE kind='tool';
CREATE INDEX governance_department_kind ON governance_records(department,kind,created_at,id);
CREATE INDEX governance_parent ON governance_records(parent_id);
CREATE TABLE governance_audit (
 id uuid PRIMARY KEY,
 at timestamptz NOT NULL,
 actor text NOT NULL,
 action text NOT NULL,
 object_id text NOT NULL,
 parent_id text NOT NULL DEFAULT '',
 department text NOT NULL,
 revision integer NOT NULL,
 reason text NOT NULL,
 result text NOT NULL CHECK(result IN ('success','denied')),
 correlation text NOT NULL,
 before_hash text NOT NULL,
 after_hash text NOT NULL
);
CREATE INDEX governance_audit_scope ON governance_audit(department,at,id);
CREATE FUNCTION governance_immutable_audit() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 RAISE EXCEPTION 'governance audit cannot be modified';
END;
$$;
CREATE TRIGGER governance_audit_immutable BEFORE UPDATE OR DELETE ON governance_audit FOR EACH ROW EXECUTE FUNCTION governance_immutable_audit();
CREATE FUNCTION governance_freeze_records() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF TG_OP='DELETE' THEN RAISE EXCEPTION 'archive governance records instead of deleting'; END IF;
 IF (OLD.kind='grant' OR (OLD.kind='tool' AND OLD.state<>'draft') OR (OLD.kind='request' AND OLD.state<>'draft')) AND
    (NEW.data IS DISTINCT FROM (OLD.data) OR NEW.parent_id IS DISTINCT FROM (OLD.parent_id) OR NEW.tool_id IS DISTINCT FROM (OLD.tool_id) OR NEW.source_id IS DISTINCT FROM (OLD.source_id) OR NEW.hash IS DISTINCT FROM (OLD.hash)) THEN
  RAISE EXCEPTION 'immutable governance snapshot';
 END IF;
 IF NEW.owner_id<>OLD.owner_id OR NEW.department<>OLD.department OR NEW.kind<>OLD.kind THEN RAISE EXCEPTION 'immutable ownership and scope'; END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER governance_records_immutable BEFORE UPDATE OR DELETE ON governance_records FOR EACH ROW EXECUTE FUNCTION governance_freeze_records();
CREATE TABLE governance_accounts (
 id text PRIMARY KEY,
 name text NOT NULL,
 department text NOT NULL,
 roles jsonb NOT NULL CHECK(jsonb_typeof(roles)='array'),
 password_hash text NOT NULL,
 revision integer NOT NULL DEFAULT 1 CHECK(revision>0)
);
