-- What ourtaiko_net must do once it holds data: the foreign keys, triggers and shared tables
-- that the two song sites relied on, now side by side. Run by db/dev/test.sh on a database
-- imported from the seeded sources, which may be empty or hold real data besides the seeds:
-- every check is about the seeded rows. Changes nothing: everything is rolled back.
\set ON_ERROR_STOP 1
BEGIN;

CREATE FUNCTION pg_temp.refused(statement text) RETURNS boolean LANGUAGE plpgsql AS $$
BEGIN
  EXECUTE statement;
  RETURN false;
EXCEPTION WHEN OTHERS THEN
  RETURN true;
END $$;

CREATE FUNCTION pg_temp.must(ok boolean, what text) RETURNS void LANGUAGE plpgsql AS $$
BEGIN
  IF ok IS NOT TRUE THEN
    RAISE EXCEPTION 'FAILED: %', what;
  END IF;
  RAISE NOTICE 'ok: %', what;
END $$;
-- Only the notices above are of interest, not each call's empty result.
\o /dev/null

-- The seeded ids (db/dev/seed-source.sql).
SELECT rpad('fc1', 32, '0') AS f_chart, rpad('dc1', 32, '0') AS o_chart,
       rpad('f51', 32, '0') AS f_score, rpad('d51', 32, '0') AS o_score,
       repeat('a', 32) AS u1, repeat('9', 32) AS newcomer \gset

-- One table of plays over both sources.
SELECT pg_temp.must((SELECT count(*) FROM fanmade.scores WHERE id LIKE 'f5_' || repeat('0', 29)) = 4
                AND (SELECT count(*) FROM official.scores WHERE id LIKE 'd5_' || repeat('0', 29)) = 4,
  'each source has its seeded scores');
SELECT pg_temp.must((SELECT count(*) FROM net.scores) = (SELECT count(*) FROM fanmade.scores) + (SELECT count(*) FROM official.scores),
  'net.scores is both sources together');
SELECT pg_temp.must((SELECT count(DISTINCT source) FROM net.scores WHERE user_id = :'u1') = 2,
  'one player''s plays of both sources come from one query');
SELECT pg_temp.must((SELECT count(*) FROM net.dan_attempts) = (SELECT count(*) FROM fanmade.dan_attempts) + (SELECT count(*) FROM official.dan_attempts)
                AND (SELECT count(DISTINCT source) FROM net.dan_attempts WHERE idempotency_key = 'seed-attempt') = 2,
  'net.dan_attempts is both sources together');
SELECT pg_temp.must((SELECT source FROM net.scores WHERE id = :'f_score') = 'fanmade' AND (SELECT source FROM net.scores WHERE id = :'o_score') = 'official',
  'each score says which source it belongs to');

-- A source's own SQL, unqualified, as the service runs it.
SET LOCAL search_path = official, net;
SELECT pg_temp.must((SELECT count(*) FROM scores s JOIN users u ON u.id = s.user_id JOIN chart_difficulties d ON d.id = s.difficulty_id WHERE d.chart_id = :'o_chart') = 3,
  'search_path "official, net" finds the source''s tables and the shared users');
RESET search_path;

-- Foreign keys from each source's scores to its own slots.
SELECT pg_temp.must(pg_temp.refused(format($q$INSERT INTO net.scores (source, id, user_id, good, ok, bad, score, drumroll, max_combo, payload_digest, difficulty_id)
  VALUES ('fanmade', %L, %L, 1, 0, 0, 1, 0, 1, %L, 424242)$q$, repeat('7', 32), :'u1', repeat('d', 64))),
  'a fanmade score for a slot that does not exist is refused');
SELECT pg_temp.must(pg_temp.refused(format($q$INSERT INTO official.scores (id, user_id, good, ok, bad, score, drumroll, max_combo, payload_digest, difficulty_id)
  VALUES (%L, %L, 1, 0, 0, 1, 0, 1, %L, 424242)$q$, repeat('7', 32), :'u1', repeat('d', 64))),
  'an official score for a slot that does not exist is refused');
SELECT pg_temp.must(pg_temp.refused('DELETE FROM fanmade.chart_difficulties WHERE id = 900001'),
  'a slot that has scores cannot be deleted');
SELECT pg_temp.must(pg_temp.refused(format($q$INSERT INTO fanmade.scores (id, user_id, good, ok, bad, score, drumroll, max_combo, payload_digest, difficulty_id)
  VALUES (%L, %L, 1, 0, 0, 1, 0, 1, %L, 900001)$q$, repeat('7', 32), :'newcomer', repeat('d', 64))),
  'a score by someone who is not in net.users is refused');

-- Slot ids are per source: the same number is a different slot in each.
INSERT INTO net.scores (source, id, user_id, good, ok, bad, score, drumroll, max_combo, payload_digest, difficulty_id)
VALUES ('official', repeat('8', 32), :'u1', 1, 0, 0, 1, 0, 1, repeat('d', 64), 900001);
SELECT pg_temp.must((SELECT count(*) FROM official.scores WHERE id = repeat('8', 32)) = 1 AND (SELECT count(*) FROM fanmade.scores WHERE id = repeat('8', 32)) = 0,
  'a score written through net.scores lands in its own source');

-- Retiring scores stays inside the source whose chart changed.
UPDATE fanmade.chart_difficulties SET content_hash = 'v2:' || repeat('9', 64) WHERE id = 900001;
SELECT pg_temp.must(NOT EXISTS (SELECT 1 FROM fanmade.scores WHERE difficulty_id = 900001 AND is_latest),
  'a changed fanmade slot retires its scores');
SELECT pg_temp.must((SELECT is_latest FROM official.scores WHERE id = :'o_score'),
  'the official slot with the same number keeps its scores');
SELECT pg_temp.must((SELECT bool_and(NOT valid) FROM fanmade.dan_course_songs WHERE chart_id = :'f_chart' AND course = 'Oni'),
  'the changed slot is marked invalid in fanmade dan courses');
SELECT pg_temp.must((SELECT bool_and(valid) FROM official.dan_course_songs WHERE course_id = rpad('dd1', 32, '0')),
  'the official dan course with the same slot numbers is untouched');
UPDATE official.charts SET audio_file = 900003 WHERE id = :'o_chart';
SELECT pg_temp.must(NOT EXISTS (SELECT 1 FROM official.scores s JOIN official.chart_difficulties d ON d.id = s.difficulty_id WHERE d.chart_id = :'o_chart' AND s.is_latest),
  'new audio on an official chart retires its scores');

-- A file a chart stops referring to is queued for deletion, in its own source.
SELECT pg_temp.must(EXISTS (SELECT 1 FROM official.retired_files WHERE storage_key = repeat('2', 64) || '.ogg'),
  'the audio an official chart no longer uses is queued for deletion');
SELECT pg_temp.must(NOT EXISTS (SELECT 1 FROM fanmade.retired_files WHERE storage_key = repeat('2', 64) || '.ogg'),
  'the fanmade file of the same name is not queued with it');
-- The same audio kept under another record is not another audio: its scores stay.
INSERT INTO fanmade.chart_resources (storage_key, original_filename, sha256, byte_size, extension)
VALUES ('legacy/seed.ogg', 'seed.ogg', repeat('2', 64), 200, 'ogg') RETURNING id AS same_audio \gset
UPDATE fanmade.charts SET audio_file = :same_audio WHERE id = :'f_chart';
SELECT pg_temp.must(EXISTS (SELECT 1 FROM fanmade.scores s JOIN fanmade.chart_difficulties d ON d.id = s.difficulty_id WHERE d.chart_id = :'f_chart' AND s.is_latest),
  'the same audio under another record keeps a fanmade chart''s scores');
UPDATE fanmade.charts SET audio_file = 900003 WHERE id = :'f_chart';
SELECT pg_temp.must(NOT EXISTS (SELECT 1 FROM fanmade.scores s JOIN fanmade.chart_difficulties d ON d.id = s.difficulty_id WHERE d.chart_id = :'f_chart' AND s.is_latest),
  'new audio on a fanmade chart retires its scores');
SELECT pg_temp.must(EXISTS (SELECT 1 FROM fanmade.retired_files WHERE storage_key = repeat('2', 64) || '.ogg'),
  'the audio a fanmade chart no longer uses is queued for deletion');

-- Players are shared; each source still guards its own charts' owners.
SELECT pg_temp.must(pg_temp.refused(format('DELETE FROM net.users WHERE id = %L', rpad('f', 32, 'e'))),
  'a player who owns a fanmade chart cannot be deleted');
SELECT pg_temp.must(pg_temp.refused(format('DELETE FROM net.users WHERE id = %L', rpad('d', 32, 'e'))),
  'a player who owns an official chart cannot be deleted');
SELECT pg_temp.must(pg_temp.refused(format('UPDATE official.charts SET owner_ids = ARRAY[%L] WHERE id = %L', :'newcomer', :'o_chart')),
  'a chart cannot be given to someone who is not in net.users');
INSERT INTO net.users (id) VALUES (:'newcomer');
UPDATE official.charts SET owner_ids = ARRAY[:'newcomer'] WHERE id = :'o_chart';
SELECT pg_temp.must((SELECT owner_ids = ARRAY[:'newcomer'] FROM official.charts WHERE id = :'o_chart'),
  'a chart can be given to a player of net.users');

-- Profile's tables point at the same players.
SELECT pg_temp.must(EXISTS (SELECT 1 FROM net.awards WHERE service = 'official' AND request_key = rpad('da1', 32, '0'))
                AND NOT EXISTS (SELECT 1 FROM net.awards WHERE service = 'database'),
  'awards asked for by OurTaiko Database are filed under official');
SELECT pg_temp.must(EXISTS (SELECT 1 FROM net.dan_attempts a JOIN net.awards w ON w.service = a.source AND w.request_key = a.id),
  'a dan attempt finds its award');
SELECT pg_temp.must(EXISTS (SELECT 1 FROM net.users WHERE id = repeat('c', 32)),
  'a player known only to Profile is in net.users');
INSERT INTO net.nameplates (user_id) VALUES (repeat('0', 32));
SELECT pg_temp.must(EXISTS (SELECT 1 FROM net.users WHERE id = repeat('0', 32)),
  'a player first heard of through a nameplate becomes a player');
SELECT pg_temp.must(pg_temp.refused(format($q$INSERT INTO net.user_titles (user_id, title_id) VALUES (%L, 'no-such-title')$q$, repeat('a', 32))),
  'a title that does not exist cannot be given');
SELECT pg_temp.must(EXISTS (SELECT 1 FROM net.services WHERE id = 'official') AND NOT EXISTS (SELECT 1 FROM net.services WHERE id = 'database'),
  'the award key of OurTaiko Database is filed under official');

-- Counters go on after the imported rows.
INSERT INTO fanmade.chart_resources (storage_key, original_filename, sha256, byte_size, extension)
VALUES (repeat('4', 64) || '.ogg', 'next.ogg', repeat('4', 64), 1, 'ogg');
SELECT pg_temp.must((SELECT max(id) FROM fanmade.chart_resources) > 900003, 'new ids continue after the imported ones');

ROLLBACK;
