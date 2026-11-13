-- Turns one song database's schema, just restored into this database as public/auth/internal,
-- into a source schema of ourtaiko_net.
--
--   CALL net_build.adopt('fanmade');   -- the first source also gives net its users and sessions
--   CALL net_build.adopt('official');
--
-- After it: every table of the source is in the schema named after it, tables that name a
-- user point at net.users, every function is pinned to "<source>, net", and scores and
-- dan_attempts carry the source column their shared parents are split by.
CREATE SCHEMA IF NOT EXISTS net_build;

CREATE OR REPLACE PROCEDURE net_build.adopt(src text) LANGUAGE plpgsql AS $$
DECLARE
  r record;
  first boolean := to_regnamespace('net') IS NULL;
  def text;
BEGIN
  IF src !~ '^[a-z][a-z0-9_]*$' THEN
    RAISE EXCEPTION 'bad source name %', src;
  END IF;
  -- Names in catalog output are always schema-qualified from here on.
  PERFORM set_config('search_path', '', true);

  EXECUTE format('ALTER SCHEMA public RENAME TO %I', src);
  CREATE SCHEMA public;

  -- Housekeeping tables join the source's own schema. Its migration history stays behind:
  -- ourtaiko_net keeps one of its own.
  FOR r IN SELECT c.relname FROM pg_catalog.pg_class c JOIN pg_catalog.pg_namespace n ON n.oid = c.relnamespace
            WHERE n.nspname = 'internal' AND c.relkind = 'r' AND c.relname <> 'schema_migrations' ORDER BY 1 LOOP
    EXECUTE format('ALTER TABLE internal.%I SET SCHEMA %I', r.relname, src);
  END LOOP;
  FOR r IN SELECT p.oid::regprocedure::text AS sig FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
            WHERE n.nspname IN ('internal', 'auth') ORDER BY 1 LOOP
    EXECUTE format('ALTER ROUTINE %s SET SCHEMA %I', r.sig, src);
  END LOOP;

  IF first THEN
    -- The site has one set of users and one sign-in: the first source's tables become net's.
    -- Everything that pointed at them keeps pointing at them.
    CREATE SCHEMA net;
    EXECUTE format('ALTER TABLE %I.users SET SCHEMA net', src);
    ALTER TABLE auth.sessions SET SCHEMA net;
    ALTER TABLE auth.oidc_flows SET SCHEMA net;
    FOR r IN SELECT tgname FROM pg_catalog.pg_trigger WHERE tgrelid = 'net.users'::regclass AND NOT tgisinternal LOOP
      EXECUTE format('ALTER TRIGGER %I ON net.users RENAME TO %I', r.tgname, src || '_' || r.tgname);
    END LOOP;
    DROP SCHEMA auth CASCADE;
  ELSE
    -- A later source: its sign-in tables go, its user references move to net.users, and the
    -- triggers it kept on its own users table are put on net.users under its name.
    DROP SCHEMA auth CASCADE;
    FOR r IN SELECT conrelid::regclass::text AS tbl, conname, pg_catalog.pg_get_constraintdef(oid) AS def
               FROM pg_catalog.pg_constraint
              WHERE contype = 'f' AND confrelid = format('%I.users', src)::regclass ORDER BY 1, 2 LOOP
      def := replace(r.def, format('REFERENCES %I.users(', src), 'REFERENCES net.users(');
      IF def = r.def THEN
        RAISE EXCEPTION 'cannot repoint % on %: %', r.conname, r.tbl, r.def;
      END IF;
      EXECUTE format('ALTER TABLE %s DROP CONSTRAINT %I', r.tbl, r.conname);
      EXECUTE format('ALTER TABLE %s ADD CONSTRAINT %I %s', r.tbl, r.conname, def);
    END LOOP;
    FOR r IN SELECT tgname, pg_catalog.pg_get_triggerdef(oid) AS def FROM pg_catalog.pg_trigger
              WHERE tgrelid = format('%I.users', src)::regclass AND NOT tgisinternal ORDER BY 1 LOOP
      def := replace(r.def, format(' ON %I.users ', src), ' ON net.users ');
      def := replace(def, format('CREATE TRIGGER %I ', r.tgname), format('CREATE TRIGGER %I ', src || '_' || r.tgname));
      IF def = r.def THEN
        RAISE EXCEPTION 'cannot move trigger %: %', r.tgname, r.def;
      END IF;
      EXECUTE def;
    END LOOP;
    EXECUTE format('DROP TABLE %I.users', src);
  END IF;
  DROP SCHEMA internal CASCADE;

  -- Function bodies are text: a schema name written inside one did not follow the rename.
  FOR r IN SELECT p.oid, pg_catalog.pg_get_functiondef(p.oid) AS def
             FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
            WHERE n.nspname = src AND p.prokind = 'f' AND p.prosrc ~ '\m(public|auth|internal)\.' ORDER BY p.oid LOOP
    def := regexp_replace(r.def, '\m(public|internal)\.', src || '.', 'g');
    def := regexp_replace(def, '\mauth\.', 'net.', 'g');
    EXECUTE def;
  END LOOP;

  -- Every function looks its tables up in its own source, then in net, whoever calls it.
  FOR r IN SELECT p.oid::regprocedure::text AS sig FROM pg_catalog.pg_proc p JOIN pg_catalog.pg_namespace n ON n.oid = p.pronamespace
            WHERE n.nspname = src ORDER BY 1 LOOP
    EXECUTE format('ALTER ROUTINE %s SET search_path = %I, net', r.sig, src);
  END LOOP;

  -- The column the shared parents in net are split by.
  EXECUTE format('ALTER TABLE %I.scores ADD COLUMN source text NOT NULL DEFAULT %L CONSTRAINT scores_source CHECK (source = %L)', src, src, src);
  EXECUTE format('ALTER TABLE %I.dan_attempts ADD COLUMN source text NOT NULL DEFAULT %L CONSTRAINT dan_attempts_source CHECK (source = %L)', src, src, src);
  EXECUTE format('COMMENT ON SCHEMA %I IS %L', src, 'Song source ' || src || ': its charts, files, dan courses and what players wrote about them.');
END $$;
