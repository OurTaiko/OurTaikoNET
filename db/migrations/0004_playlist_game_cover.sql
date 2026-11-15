-- The game draws a playlist's WebP cover itself (OurTaikoPlay decodes WebP), so the small JPEG
-- copy that was made for it (game_cover_file) is no longer made, stored or linked. The release
-- before this one still reads the column, so it stays for now, empty: emptying it queues the
-- JPEG files for deletion (retire_playlist_cover), and it no longer has to be set together
-- with cover_file. A later change drops it.
ALTER TABLE fanmade.playlists DROP CONSTRAINT IF EXISTS playlists_check;
ALTER TABLE official.playlists DROP CONSTRAINT IF EXISTS playlists_check;
UPDATE fanmade.playlists SET game_cover_file = NULL WHERE game_cover_file IS NOT NULL;
UPDATE official.playlists SET game_cover_file = NULL WHERE game_cover_file IS NOT NULL;
COMMENT ON COLUMN fanmade.playlists.game_cover_file IS 'Unused since migration 0004 (the game reads cover_file); always null, to be dropped.';
COMMENT ON COLUMN official.playlists.game_cover_file IS 'Unused since migration 0004 (the game reads cover_file); always null, to be dropped.';
