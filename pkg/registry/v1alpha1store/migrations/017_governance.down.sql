DROP TABLE IF EXISTS governance_accounts;
DROP TABLE IF EXISTS governance_audit;
DROP TABLE IF EXISTS governance_records;
DROP FUNCTION IF EXISTS governance_immutable_audit();
DROP FUNCTION IF EXISTS governance_freeze_records();
DROP INDEX IF EXISTS agents_governance_uid;
DROP INDEX IF EXISTS mcp_servers_governance_uid;
