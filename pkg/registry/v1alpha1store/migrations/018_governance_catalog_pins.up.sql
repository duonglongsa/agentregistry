-- Pin referenced catalog versions even if the application is later restarted in
-- public mode. Status-only controller updates remain allowed; semantic changes
-- require a new catalog tag/snapshot and review.
CREATE FUNCTION governance_catalog_pin() RETURNS trigger LANGUAGE plpgsql AS $$
BEGIN
 IF EXISTS (SELECT 1 FROM governance_records WHERE agent_uid=OLD.uid OR server_uid=OLD.uid) AND
    (NEW.spec<>OLD.spec OR NEW.content_hash<>OLD.content_hash OR NEW.namespace<>OLD.namespace OR NEW.name<>OLD.name OR NEW.tag<>OLD.tag OR NEW.uid<>OLD.uid OR NEW.deletion_timestamp IS DISTINCT FROM (OLD.deletion_timestamp)) THEN
  RAISE EXCEPTION 'catalog version is pinned by governance; create a new tag or archive through governance';
 END IF;
 RETURN NEW;
END;
$$;
CREATE TRIGGER agents_governance_pin BEFORE UPDATE ON agents FOR EACH ROW EXECUTE FUNCTION governance_catalog_pin();
CREATE TRIGGER mcp_servers_governance_pin BEFORE UPDATE ON mcp_servers FOR EACH ROW EXECUTE FUNCTION governance_catalog_pin();
