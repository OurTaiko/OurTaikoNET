-- A playlist may name the play backdrop the game uses for songs opened from it (a stage preset,
-- as a chart's SCENEPRESET does). The server keeps the name as its owner chose it and does not
-- know which presets exist: they are published with the game's stage files
-- (distrib.ourtaiko.org/stage/catalog.json), and a game passes over a name it does not have.
-- Empty for none.
ALTER TABLE fanmade.playlists ADD COLUMN IF NOT EXISTS scene_preset text NOT NULL DEFAULT ''
    CONSTRAINT playlists_scene_preset_check CHECK (scene_preset ~ '^[A-Za-z0-9_]{0,32}$');
ALTER TABLE official.playlists ADD COLUMN IF NOT EXISTS scene_preset text NOT NULL DEFAULT ''
    CONSTRAINT playlists_scene_preset_check CHECK (scene_preset ~ '^[A-Za-z0-9_]{0,32}$');
COMMENT ON COLUMN fanmade.playlists.scene_preset IS 'The stage preset the game plays this playlist''s songs on; empty for none. Not checked against the published presets.';
COMMENT ON COLUMN official.playlists.scene_preset IS 'The stage preset the game plays this playlist''s songs on; empty for none. Not checked against the published presets.';
