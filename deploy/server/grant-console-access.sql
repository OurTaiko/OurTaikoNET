-- Lets the two accounts the database console (CloudBeaver, postgre.ourtaiko.org) signs in
-- with use ourtaiko_net, as they used the old sites' databases. Run as the superuser, in
-- ourtaiko_net; it can be repeated.
--
--   ourtaiko_viewer           reads everything (its role is read-only by its own settings)
--   ourtaiko_database_editor  reads everything, and writes the official source: the schema
--                             `official`, as it wrote OurTaiko Database's own database
--
-- Both already exist with the passwords the console has. Neither may change the schema.
-- What a later migration creates is covered by the default privileges at the end, which are
-- for objects made by ourtaiko_net, the role the server runs and migrates as.
\set ON_ERROR_STOP 1
BEGIN;

GRANT CONNECT ON DATABASE ourtaiko_net TO ourtaiko_viewer, ourtaiko_database_editor;
GRANT USAGE ON SCHEMA fanmade, official, net TO ourtaiko_viewer, ourtaiko_database_editor;
GRANT SELECT ON ALL TABLES IN SCHEMA fanmade, official, net TO ourtaiko_viewer, ourtaiko_database_editor;

GRANT INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA official TO ourtaiko_database_editor;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA official TO ourtaiko_database_editor;

ALTER DEFAULT PRIVILEGES FOR ROLE ourtaiko_net IN SCHEMA fanmade, official, net
  GRANT SELECT ON TABLES TO ourtaiko_viewer, ourtaiko_database_editor;
ALTER DEFAULT PRIVILEGES FOR ROLE ourtaiko_net IN SCHEMA official
  GRANT INSERT, UPDATE, DELETE ON TABLES TO ourtaiko_database_editor;
ALTER DEFAULT PRIVILEGES FOR ROLE ourtaiko_net IN SCHEMA official
  GRANT USAGE, SELECT ON SEQUENCES TO ourtaiko_database_editor;

-- The editor's own search path names the old database's schemas. Here a name without a
-- schema is the official source's, then the shared one, as for the server's own connection
-- to that source. The viewer has none: it says which source it means.
ALTER ROLE ourtaiko_database_editor IN DATABASE ourtaiko_net SET search_path = official, net;

COMMIT;
