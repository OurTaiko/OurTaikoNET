-- A title's look is the three layers a nameplate wears for it (OurTaikoPlay's NameplateLayers):
-- a band, an effect over it and an icon on its end. Each is an item of the public nameplate
-- catalog (nameplate/lumen.json), named '<slot>/<name>'; a title has a band and may have no
-- effect or icon. Whoever makes a title picks them; a player wears only titles they hold.
--
-- style stays for the game versions released before the layers, which know six bands and read
-- nothing else: a title kept from then keeps its own, and the server gives a new or changed
-- title the band of the six nearest its own (internal/profile/httpapi, legacyStyle).
ALTER TABLE net.titles
    ADD COLUMN IF NOT EXISTS band_item text NOT NULL DEFAULT 'band/lumen-title-wood'
        CONSTRAINT titles_band_item CHECK (band_item ~ '^band/[a-z0-9][a-z0-9_.-]{0,63}$'),
    ADD COLUMN IF NOT EXISTS effect_item text
        CONSTRAINT titles_effect_item CHECK (effect_item ~ '^effect/[a-z0-9][a-z0-9_.-]{0,63}$'),
    ADD COLUMN IF NOT EXISTS icon_item text
        CONSTRAINT titles_icon_item CHECK (icon_item ~ '^icon/[a-z0-9][a-z0-9_.-]{0,63}$');

-- The titles made before look as their band did: the catalog's title of the same name (gold and
-- purple glint), and wood for the two bands the catalog has no title for (teal, wood2).
UPDATE net.titles SET
    band_item = 'band/lumen-title-' || CASE WHEN style IN ('gold', 'purple', 'rainbow') THEN style ELSE 'wood' END,
    effect_item = CASE WHEN style IN ('gold', 'purple') THEN 'effect/lumen-title-' || style END,
    icon_item = NULL;

COMMENT ON COLUMN net.titles.style IS 'The band of the six (wood, teal, wood2, purple, gold, rainbow) that game versions released before the layers draw. New and changed titles get the one nearest their band_item.';
COMMENT ON COLUMN net.titles.band_item IS 'The band a nameplate wears for this title: an item of the public nameplate catalog, band/<name>.';
COMMENT ON COLUMN net.titles.effect_item IS 'The effect drawn over the band (effect/<name>), or none.';
COMMENT ON COLUMN net.titles.icon_item IS 'The icon on the band''s end (icon/<name>), or none.';

-- A title given is shown as it looks, layers and all.
CREATE OR REPLACE FUNCTION net.reward_title() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  INSERT INTO rewards(user_id, kind, data)
  SELECT NEW.user_id, 'title', jsonb_build_object(
    'title', jsonb_build_object('id', t.id, 'name', t.name, 'nameTranslations', t.name_translations,
      'description', t.description, 'style', t.style, 'official', t.official, 'ownerId', t.owner_id,
      'layers', jsonb_build_object('band', t.band_item, 'effect', t.effect_item, 'icon', t.icon_item)),
    'service', a.service, 'courseTitle', a.course_title)
  FROM titles t LEFT JOIN awards a ON a.id = NEW.award_id
  WHERE t.id = NEW.title_id;
  RETURN NULL;
END $$;

-- Titles given before and not seen yet get their layers too.
UPDATE net.rewards r SET data = jsonb_set(r.data, '{title,layers}',
    jsonb_build_object('band', t.band_item, 'effect', t.effect_item, 'icon', t.icon_item))
FROM net.titles t
WHERE r.kind = 'title' AND r.seen_at IS NULL AND r.data #>> '{title,id}' = t.id;
