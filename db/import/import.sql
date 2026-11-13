-- Fills ourtaiko_net from the three databases it replaces. Run by db/import.sh, which sets
--   fanmade_db, official_db, profile_db, src_host, src_port, src_user, src_password.
--
-- The whole import is one transaction: it either replaces everything or changes nothing.
-- It can be run again at any time. Each run replaces all song and player data with the
-- sources' current state; net.roles and net.sessions are left alone, and net.users only
-- gains rows.
--
-- It stops with an error when a source has a table or column this database does not know,
-- when a foreign key does not hold afterwards, or when a table's row count or content
-- differs from its source.
\set ON_ERROR_STOP 1
BEGIN;

CREATE EXTENSION IF NOT EXISTS postgres_fdw WITH SCHEMA public;
DROP SCHEMA IF EXISTS net_import CASCADE;
DROP SERVER IF EXISTS old_fanmade CASCADE;
DROP SERVER IF EXISTS old_official CASCADE;
DROP SERVER IF EXISTS old_profile CASCADE;
CREATE SCHEMA net_import;
CREATE SERVER old_fanmade FOREIGN DATA WRAPPER postgres_fdw OPTIONS (host :'src_host', port :'src_port', dbname :'fanmade_db');
CREATE SERVER old_official FOREIGN DATA WRAPPER postgres_fdw OPTIONS (host :'src_host', port :'src_port', dbname :'official_db');
CREATE SERVER old_profile FOREIGN DATA WRAPPER postgres_fdw OPTIONS (host :'src_host', port :'src_port', dbname :'profile_db');
CREATE USER MAPPING FOR CURRENT_USER SERVER old_fanmade OPTIONS (user :'src_user', password :'src_password');
CREATE USER MAPPING FOR CURRENT_USER SERVER old_official OPTIONS (user :'src_user', password :'src_password');
CREATE USER MAPPING FOR CURRENT_USER SERVER old_profile OPTIONS (user :'src_user', password :'src_password');

-- Each old database as foreign tables: its public and internal tables side by side.
CREATE SCHEMA old_fanmade;
CREATE SCHEMA old_official;
CREATE SCHEMA old_profile;
IMPORT FOREIGN SCHEMA public FROM SERVER old_fanmade INTO old_fanmade;
IMPORT FOREIGN SCHEMA internal EXCEPT (schema_migrations) FROM SERVER old_fanmade INTO old_fanmade;
IMPORT FOREIGN SCHEMA public FROM SERVER old_official INTO old_official;
IMPORT FOREIGN SCHEMA internal EXCEPT (schema_migrations) FROM SERVER old_official INTO old_official;
IMPORT FOREIGN SCHEMA public FROM SERVER old_profile INTO old_profile;

CREATE TABLE net_import.report (
  target text PRIMARY KEY,
  origin text NOT NULL,
  origin_rows bigint,
  target_rows bigint,
  same_content boolean
);

-- The columns of a target table that its origin also has, in the target's order.
CREATE FUNCTION net_import.shared_columns(target regclass, origin regclass) RETURNS text LANGUAGE sql STABLE AS $$
  SELECT string_agg(quote_ident(t.attname), ', ' ORDER BY t.attnum)
    FROM pg_attribute t
    JOIN pg_attribute o ON o.attrelid = origin AND o.attname = t.attname AND o.attnum > 0 AND NOT o.attisdropped
   WHERE t.attrelid = target AND t.attnum > 0 AND NOT t.attisdropped AND t.attgenerated = ''
$$;

-- Row count and an order-independent digest of the given columns.
CREATE FUNCTION net_import.digest(rel text, cols text, OUT n bigint, OUT d text) LANGUAGE plpgsql AS $$
BEGIN
  EXECUTE format('SELECT count(*), md5(COALESCE(string_agg(h, '''' ORDER BY h), '''')) FROM (SELECT md5(ROW(%s)::text) AS h FROM %s) s', cols, rel)
     INTO n, d;
END $$;

-- Copies one table and records how it compares with its origin. map, when given, is a
-- select list that replaces the plain column list on the origin's side.
CREATE PROCEDURE net_import.copy_table(target regclass, origin regclass, map text DEFAULT NULL) LANGUAGE plpgsql AS $$
DECLARE
  cols text := net_import.shared_columns(target, origin);
  extra text;
  o record;
  t record;
BEGIN
  SELECT string_agg(a.attname, ', ') INTO extra FROM pg_attribute a
   WHERE a.attrelid = origin AND a.attnum > 0 AND NOT a.attisdropped
     AND NOT EXISTS (SELECT 1 FROM pg_attribute x WHERE x.attrelid = target AND x.attname = a.attname AND NOT x.attisdropped);
  IF extra IS NOT NULL THEN
    RAISE EXCEPTION '% has columns that % does not: %', origin, target, extra;
  END IF;
  EXECUTE format('INSERT INTO %s (%s) OVERRIDING SYSTEM VALUE SELECT %s FROM %s', target, cols, COALESCE(map, cols), origin);
  SELECT * INTO o FROM net_import.digest(format('(SELECT %s FROM %s) q', COALESCE(map, cols), origin), cols);
  SELECT * INTO t FROM net_import.digest(target::text, cols);
  INSERT INTO net_import.report VALUES (target::text, origin::text, o.n, t.n, o.d = t.d);
END $$;

CREATE PROCEDURE net_import.run() LANGUAGE plpgsql AS $$
DECLARE
  src text;
  r record;
  list text;
  handled CONSTANT text[] := ARRAY['users', 'sessions', 'oidc_flows'];
BEGIN
  -- Rows arrive exactly as they are: no trigger recounts votes or touches update_at, and
  -- foreign keys are checked afterwards by db/check.sql instead of row by row.
  SET LOCAL session_replication_role = replica;

  -- Nothing may be in a source that this database has no place for.
  FOREACH src IN ARRAY ARRAY['fanmade', 'official'] LOOP
    SELECT string_agg(c.relname, ', ') INTO list FROM pg_class c
     WHERE c.relnamespace = ('old_' || src)::regnamespace AND c.relkind = 'f' AND c.relname <> ALL (handled)
       AND to_regclass(format('%I.%I', src, c.relname)) IS NULL;
    IF list IS NOT NULL THEN
      RAISE EXCEPTION 'the % database has tables ourtaiko_net does not know: %', src, list;
    END IF;
  END LOOP;

  -- Empty every table that is about to be refilled.
  SELECT string_agg(format('%I.%I', n.nspname, c.relname), ', ') INTO list
    FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
   WHERE c.relkind = 'r'
     AND (n.nspname IN ('fanmade', 'official')
          OR (n.nspname = 'net' AND c.relname IN ('titles', 'awards', 'user_titles', 'user_dans', 'nameplates', 'services')));
  EXECUTE 'TRUNCATE ' || list;

  -- Players: everyone either song site has seen, and everyone Profile holds something for.
  INSERT INTO net.users AS u (id, first_login_at, last_active_at)
  SELECT id, min(first_login_at), max(last_active_at)
    FROM (SELECT id, first_login_at, last_active_at FROM old_fanmade.users
          UNION ALL SELECT id, first_login_at, last_active_at FROM old_official.users
          UNION ALL SELECT user_id, NULL, NULL FROM old_profile.awards
          UNION ALL SELECT user_id, NULL, NULL FROM old_profile.user_titles
          UNION ALL SELECT user_id, NULL, NULL FROM old_profile.user_dans
          UNION ALL SELECT user_id, NULL, NULL FROM old_profile.nameplates
          UNION ALL SELECT owner_id, NULL, NULL FROM old_profile.titles WHERE owner_id IS NOT NULL) seen
   GROUP BY id
  ON CONFLICT (id) DO UPDATE
     SET first_login_at = LEAST(u.first_login_at, EXCLUDED.first_login_at),
         last_active_at = GREATEST(u.last_active_at, EXCLUDED.last_active_at);
  INSERT INTO net_import.report
  SELECT 'net.users', 'users of all three',
         (SELECT count(*) FROM (SELECT id FROM old_fanmade.users UNION SELECT id FROM old_official.users
                                UNION SELECT user_id FROM old_profile.awards UNION SELECT user_id FROM old_profile.user_titles
                                UNION SELECT user_id FROM old_profile.user_dans UNION SELECT user_id FROM old_profile.nameplates
                                UNION SELECT owner_id FROM old_profile.titles WHERE owner_id IS NOT NULL) ids),
         (SELECT count(*) FROM net.users), NULL;

  -- Each source: every table as it is.
  FOREACH src IN ARRAY ARRAY['fanmade', 'official'] LOOP
    FOR r IN SELECT c.oid::regclass AS target, c.relname FROM pg_class c
              WHERE c.relnamespace = src::regnamespace AND c.relkind = 'r' ORDER BY c.relname LOOP
      IF to_regclass(format('%I.%I', 'old_' || src, r.relname)) IS NULL THEN
        RAISE EXCEPTION 'the % database has no table %', src, r.relname;
      END IF;
      CALL net_import.copy_table(r.target, format('%I.%I', 'old_' || src, r.relname)::regclass);
    END LOOP;
  END LOOP;

  -- Profile. Its awards name the song site that asked for them; OurTaiko Database is the
  -- source called official here.
  CALL net_import.copy_table('net.services', 'old_profile.services',
    'CASE id WHEN ''database'' THEN ''official'' ELSE id END, name, key_hash, enabled, created_at');
  CALL net_import.copy_table('net.titles', 'old_profile.titles');
  CALL net_import.copy_table('net.awards', 'old_profile.awards',
    'id, CASE service WHEN ''database'' THEN ''official'' ELSE service END, request_key, user_id, course_id, course_title, official, verdict, dan, granted, created_at');
  CALL net_import.copy_table('net.user_titles', 'old_profile.user_titles');
  CALL net_import.copy_table('net.user_dans', 'old_profile.user_dans');
  CALL net_import.copy_table('net.nameplates', 'old_profile.nameplates');

  -- Counters continue after the highest number copied.
  FOR r IN SELECT format('%I.%I', n.nspname, c.relname) AS tbl, a.attname AS col,
                  pg_get_serial_sequence(format('%I.%I', n.nspname, c.relname), a.attname) AS seq
             FROM pg_class c JOIN pg_namespace n ON n.oid = c.relnamespace
             JOIN pg_attribute a ON a.attrelid = c.oid AND a.attnum > 0 AND NOT a.attisdropped
            WHERE c.relkind = 'r' AND n.nspname IN ('fanmade', 'official', 'net')
              AND pg_get_serial_sequence(format('%I.%I', n.nspname, c.relname), a.attname) IS NOT NULL LOOP
    EXECUTE format('SELECT setval(%L, COALESCE(max(%I), 1), max(%I) IS NOT NULL) FROM %s', r.seq, r.col, r.col, r.tbl);
  END LOOP;
END $$;

CALL net_import.run();

\echo
\echo 'Tables that differ from their origin (none expected):'
SELECT * FROM net_import.report WHERE target_rows IS DISTINCT FROM origin_rows OR same_content IS FALSE ORDER BY target;
\echo 'Imported:'
SELECT count(*) AS tables, sum(target_rows) AS total_rows,
       sum(target_rows) FILTER (WHERE target LIKE 'fanmade.%') AS fanmade_rows,
       sum(target_rows) FILTER (WHERE target LIKE 'official.%') AS official_rows,
       sum(target_rows) FILTER (WHERE target LIKE 'net.%') AS net_rows
  FROM net_import.report;

DO $$
BEGIN
  IF EXISTS (SELECT 1 FROM net_import.report WHERE target_rows IS DISTINCT FROM origin_rows OR same_content IS FALSE) THEN
    RAISE EXCEPTION 'import does not match its origin; nothing was changed';
  END IF;
END $$;

-- Keep the report, drop the doors to the old databases.
CREATE TABLE IF NOT EXISTS net.import_runs (
  finished_at timestamptz PRIMARY KEY DEFAULT now(),
  report jsonb NOT NULL
);
INSERT INTO net.import_runs (report) SELECT jsonb_agg(to_jsonb(r) ORDER BY r.target) FROM net_import.report r;
DROP SERVER old_fanmade CASCADE;
DROP SERVER old_official CASCADE;
DROP SERVER old_profile CASCADE;
DROP SCHEMA old_fanmade, old_official, old_profile, net_import CASCADE;

COMMIT;
