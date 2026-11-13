-- Structural rules of ourtaiko_net. Fails if one is broken. Run after a build and after an import.
\set ON_ERROR_STOP 1
DO $$
DECLARE
  r record;
  n bigint;
  bad text := '';
BEGIN
  -- 1. Every function of a source is pinned to "<source>, net". One that is not looks its
  --    tables up in the caller's search_path and fails, or worse, finds the other source's.
  FOR r IN SELECT p.oid::regprocedure::text AS sig, ns.nspname
             FROM pg_proc p JOIN pg_namespace ns ON ns.oid = p.pronamespace
            WHERE ns.nspname IN ('fanmade', 'official')
              AND NOT COALESCE(p.proconfig @> ARRAY[format('search_path=%s, net', ns.nspname)], false) LOOP
    bad := bad || format(E'\n  function %s is not pinned to "%s, net"', r.sig, r.nspname);
  END LOOP;

  -- 2. No function body names a schema that does not exist here.
  FOR r IN SELECT p.oid::regprocedure::text AS sig FROM pg_proc p JOIN pg_namespace ns ON ns.oid = p.pronamespace
            WHERE ns.nspname IN ('fanmade', 'official', 'net') AND p.prosrc ~ '\m(public|auth|internal)\.' LOOP
    bad := bad || format(E'\n  function %s still names public, auth or internal', r.sig);
  END LOOP;

  -- 3. The two sources have the same tables.
  FOR r IN SELECT COALESCE(f.relname, o.relname) AS t, f.relname IS NULL AS no_fanmade
             FROM (SELECT relname FROM pg_class WHERE relnamespace = 'fanmade'::regnamespace AND relkind = 'r') f
             FULL JOIN (SELECT relname FROM pg_class WHERE relnamespace = 'official'::regnamespace AND relkind = 'r') o USING (relname)
            WHERE f.relname IS NULL OR o.relname IS NULL LOOP
    bad := bad || format(E'\n  table %s is missing from %s', r.t, CASE WHEN r.no_fanmade THEN 'fanmade' ELSE 'official' END);
  END LOOP;

  -- 4. No source keeps users or sign-in tables of its own.
  FOR r IN SELECT ns.nspname, c.relname FROM pg_class c JOIN pg_namespace ns ON ns.oid = c.relnamespace
            WHERE ns.nspname IN ('fanmade', 'official') AND c.relname IN ('users', 'sessions', 'oidc_flows', 'schema_migrations') LOOP
    bad := bad || format(E'\n  %s.%s should not exist', r.nspname, r.relname);
  END LOOP;

  -- 5. Both partitions of each shared parent are attached.
  FOR r IN SELECT p.t FROM (VALUES ('scores'), ('dan_attempts')) p(t)
            WHERE (SELECT count(*) FROM pg_inherits WHERE inhparent = format('net.%I', p.t)::regclass) <> 2 LOOP
    bad := bad || format(E'\n  net.%s does not have both sources attached', r.t);
  END LOOP;

  -- 6. Every foreign key holds. Imports load with triggers off, so this is checked by query.
  FOR r IN
    SELECT c.conname, c.conrelid::regclass::text AS child, c.confrelid::regclass::text AS parent,
           (SELECT string_agg(format('c.%I IS NOT NULL', a.attname), ' AND ' ORDER BY k.ord)
              FROM unnest(c.conkey) WITH ORDINALITY k(attnum, ord) JOIN pg_attribute a ON a.attrelid = c.conrelid AND a.attnum = k.attnum) AS present,
           (SELECT string_agg(format('p.%I = c.%I', pa.attname, ca.attname), ' AND ' ORDER BY k.ord)
              FROM unnest(c.conkey, c.confkey) WITH ORDINALITY k(cattnum, pattnum, ord)
              JOIN pg_attribute ca ON ca.attrelid = c.conrelid AND ca.attnum = k.cattnum
              JOIN pg_attribute pa ON pa.attrelid = c.confrelid AND pa.attnum = k.pattnum) AS matches
      FROM pg_constraint c JOIN pg_class rel ON rel.oid = c.conrelid
     WHERE c.contype = 'f' AND c.connamespace IN ('fanmade'::regnamespace, 'official'::regnamespace, 'net'::regnamespace)
       AND rel.relkind = 'r' AND c.conparentid = 0
  LOOP
    EXECUTE format('SELECT count(*) FROM %s c WHERE %s AND NOT EXISTS (SELECT 1 FROM %s p WHERE %s)', r.child, r.present, r.parent, r.matches) INTO n;
    IF n > 0 THEN
      bad := bad || format(E'\n  %s rows of %s break %s (-> %s)', n, r.child, r.conname, r.parent);
    END IF;
  END LOOP;

  -- 7. Each source queues the files its charts and playlists stop referring to, and retires
  --    a chart's scores when its audio is replaced.
  FOR r IN SELECT s.nspname, t.rel, t.tgname FROM (VALUES ('fanmade'), ('official')) s(nspname),
                  (VALUES ('charts', 'charts_retire_files'), ('charts', 'charts_retire_scores'), ('playlists', 'playlists_retire_cover')) t(rel, tgname)
            WHERE NOT EXISTS (SELECT 1 FROM pg_trigger g WHERE g.tgrelid = format('%I.%I', s.nspname, t.rel)::regclass AND g.tgname = t.tgname AND g.tgenabled <> 'D') LOOP
    bad := bad || format(E'\n  %s.%s has no trigger %s', r.nspname, r.rel, r.tgname);
  END LOOP;

  IF bad <> '' THEN
    RAISE EXCEPTION 'ourtaiko_net check failed:%', bad;
  END IF;
  RAISE NOTICE 'ourtaiko_net check passed';
END $$;
