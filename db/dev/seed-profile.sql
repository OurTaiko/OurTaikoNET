-- Test rows for a local copy of OurTaikoProfile's database. Never run this on a real one.
-- Player c… exists only here: neither song site has seen them.
\set ON_ERROR_STOP 1
BEGIN;
INSERT INTO titles (id, name, style, official, owner_id) VALUES
  ('seed-official', 'Seed official', 'gold', true, NULL),
  ('seed-own', 'Seed community', 'teal', false, repeat('a', 32));
INSERT INTO awards (service, request_key, user_id, course_id, course_title, official, verdict, dan, granted) VALUES
  ('fanmade', rpad('fa1', 32, '0'), repeat('a', 32), rpad('fd1', 32, '0'), 'Seed course', true, 4, 10, '{"dan":true}'),
  ('database', rpad('da1', 32, '0'), repeat('a', 32), rpad('dd1', 32, '0'), 'Seed course', false, 4, NULL, '{}'),
  ('admin', 'seed-grant', repeat('c', 32), '', '', true, 0, NULL, '{}');
INSERT INTO user_titles (user_id, title_id, award_id) VALUES
  (repeat('a', 32), 'seed-official', (SELECT id FROM awards WHERE service = 'fanmade' AND request_key = rpad('fa1', 32, '0'))),
  (repeat('c', 32), 'seed-own', (SELECT id FROM awards WHERE service = 'admin' AND request_key = 'seed-grant'));
INSERT INTO user_dans (user_id, dan, verdict, award_id) VALUES
  (repeat('a', 32), 10, 4, (SELECT id FROM awards WHERE service = 'fanmade' AND request_key = rpad('fa1', 32, '0')));
INSERT INTO nameplates (user_id, title_id) VALUES (repeat('a', 32), 'seed-official'), (repeat('c', 32), NULL);
COMMIT;
