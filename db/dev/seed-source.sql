-- Test rows for a local copy of a song database (Fanmade's or OurTaiko Database's, in their
-- own public/auth/internal layout), so an import has something of every kind to carry.
-- Never run this on a real database.
--
--   psql -v tag=f <fanmade copy> -f seed-source.sql
--   psql -v tag=d <database copy> -f seed-source.sql
--
-- tag (one hex digit) keeps the two sources' ids apart; players a… and b… are in both.
\set ON_ERROR_STOP 1
BEGIN;
-- Rows go in as finished facts; the sites' triggers would recount and re-stamp them.
SET LOCAL session_replication_role = replica;
SET LOCAL search_path = public, internal, auth;

SELECT repeat('a', 32) AS u1, repeat('b', 32) AS u2, rpad(:'tag', 32, 'e') AS u3,
       rpad(:'tag' || 'c1', 32, '0') AS chart1, rpad(:'tag' || 'c2', 32, '0') AS chart2,
       rpad(:'tag' || 'd1', 32, '0') AS dan1, rpad(:'tag' || 'a1', 32, '0') AS attempt1,
       rpad(:'tag' || '51', 32, '0') AS score1, rpad(:'tag' || '52', 32, '0') AS score2,
       rpad(:'tag' || '53', 32, '0') AS score3, rpad(:'tag' || '54', 32, '0') AS score4,
       rpad(:'tag' || 'b1', 32, '0') AS comment1, rpad(:'tag' || 'b2', 32, '0') AS comment2,
       rpad(:'tag' || 'b3', 32, '0') AS comment3, rpad(:'tag' || 'e1', 32, '0') AS playlist1,
       rpad(:'tag' || 'f1', 32, '0') AS note1, rpad(:'tag' || 'cc', 32, '0') AS collab1 \gset

INSERT INTO users (id, first_login_at, last_active_at) VALUES
  (:'u1', '2026-01-01', '2026-06-01'), (:'u2', '2026-02-01', '2026-03-01'), (:'u3', NULL, NULL)
ON CONFLICT (id) DO NOTHING;

INSERT INTO chart_resources (id, storage_key, original_filename, sha256, byte_size, extension) OVERRIDING SYSTEM VALUE VALUES
  (900001, repeat('1', 64) || '.tja', 'seed.tja', repeat('1', 64), 100, 'tja'),
  (900002, repeat('2', 64) || '.ogg', 'seed.ogg', repeat('2', 64), 200, 'ogg'),
  (900003, repeat('3', 64) || '.ogg', 'seed2.ogg', repeat('3', 64), 300, 'ogg');
INSERT INTO charts (id, owner_ids, title, bpm, duration, demo_end, tja_file, audio_file, number_id) OVERRIDING SYSTEM VALUE VALUES
  (:'chart1', ARRAY[:'u1'], 'Seed One ' || :'tag', 120, 60, 10, 900001, 900002, 900001),
  (:'chart2', ARRAY[:'u2', :'u3'], 'Seed Two ' || :'tag', 180, 90, 20, 900001, 900002, 900002);
INSERT INTO chart_difficulties (id, chart_id, course, side, level, content_hash) OVERRIDING SYSTEM VALUE VALUES
  (900001, :'chart1', 'Oni', 'single', 8, 'v2:' || repeat('0', 64)),
  (900002, :'chart1', 'Hard', 'single', 5, 'v2:' || repeat('1', 64)),
  (900003, :'chart2', 'Oni', 'single', 10, 'v2:' || repeat('2', 64));
INSERT INTO chart_stats (chart_id) VALUES (:'chart1'), (:'chart2') ON CONFLICT DO NOTHING;

INSERT INTO scores (id, user_id, good, ok, bad, score, drumroll, max_combo, payload_digest, difficulty_id, is_latest, idempotency_key, replay_data, submitted_at) VALUES
  (:'score1', :'u1', 400, 20, 3, 987650, 12, 380, repeat('d', 64), 900001, true, 'seed-1', '{"version":1,"audio_offset_ms":0,"visual_offset_ms":0,"inputs":[[1234.5,1]]}', '2026-05-01 10:00+00'),
  (:'score2', :'u1', 300, 90, 30, 700000, 4, 120, repeat('d', 64), 900001, false, 'seed-2', NULL, '2026-04-01 10:00+00'),
  (:'score3', :'u2', 410, 10, 0, 1001000, 20, 420, repeat('d', 64), 900003, true, 'seed-3', NULL, '2026-05-02 10:00+00'),
  (:'score4', :'u3', 100, 50, 80, 300000, 0, 40, repeat('d', 64), 900002, true, NULL, NULL, '2026-05-03 10:00+00');

INSERT INTO dan_courses (id, owner_id, title, exams, code) VALUES
  (:'dan1', :'u1', '{"en":"Seed course"}', '[]', 'SEED0' || upper(:'tag'));
INSERT INTO dan_course_songs (course_id, position, chart_id, course, side) VALUES
  (:'dan1', 0, :'chart1', 'Oni', 'single'), (:'dan1', 1, :'chart2', 'Oni', 'single');
INSERT INTO dan_attempts (id, course_id, user_id, idempotency_key, payload_digest, verdict, songs_played, submitted_at) VALUES
  (:'attempt1', :'dan1', :'u1', 'seed-attempt', repeat('d', 64), 4, ARRAY[:'score1'], '2026-05-01 10:05+00');
INSERT INTO dan_votes (dan_id, user_id, value) VALUES (:'dan1', :'u2', 1);

INSERT INTO comments (id, root_id, depth, author_id, body, chart_id) VALUES (:'comment1', :'comment1', 0, :'u1', 'first', :'chart1');
INSERT INTO comments (id, root_id, parent_id, depth, author_id, body, chart_id) VALUES (:'comment2', :'comment1', :'comment1', 1, :'u2', 'reply', :'chart1');
INSERT INTO comments (id, root_id, depth, author_id, body, dan_id) VALUES (:'comment3', :'comment3', 0, :'u2', 'on a course', :'dan1');
INSERT INTO comment_votes (comment_id, user_id, value) VALUES (:'comment1', :'u2', 1);
INSERT INTO chart_votes (chart_id, user_id, value) VALUES (:'chart1', :'u2', 1), (:'chart2', :'u1', -1);

INSERT INTO notifications (id, recipient_id, actor_id, kind) VALUES (:'note1', :'u1', :'u2', 'comment_reply');
INSERT INTO notification_mutes (user_id, kind) VALUES (:'u2', 'comment_reply');

INSERT INTO playlists (id, owner_id, name) VALUES (:'playlist1', :'u1', 'Seed list');
INSERT INTO playlist_items (playlist_id, chart_id, position) VALUES (:'playlist1', :'chart1', 0), (:'playlist1', :'chart2', 1);
INSERT INTO playlist_favorites (playlist_id, user_id) VALUES (:'playlist1', :'u2');

INSERT INTO chart_collaborations (id, chart_id, inviter_id, recipient_id) VALUES (:'collab1', :'chart1', :'u1', :'u2');
INSERT INTO retired_score_requests (user_id, idempotency_key) VALUES (:'u1', 'seed-retired');
INSERT INTO upload_requests (user_id, idempotency_key, payload_digest) VALUES (:'u1', 'seed-upload', repeat('d', 64));

COMMIT;
