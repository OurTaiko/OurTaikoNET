-- A title says whether it is private or public by its visibility alone. Until now a title given
-- was also told with official (true for a public title), for the website from before visibility;
-- nothing reads it any more, and a title's JSON no longer has it (internal/profile/httpapi).
CREATE OR REPLACE FUNCTION net.reward_title() RETURNS trigger LANGUAGE plpgsql SET search_path = net AS $$
BEGIN
  INSERT INTO rewards(user_id, kind, data)
  SELECT NEW.user_id, 'title', jsonb_build_object(
    'title', jsonb_build_object('id', t.id, 'name', t.name, 'nameTranslations', t.name_translations,
      'description', t.description, 'obtain', t.obtain, 'style', t.style,
      'visibility', t.visibility, 'ownerId', t.owner_id,
      'layers', jsonb_build_object('band', t.band_item, 'effect', t.effect_item, 'icon', t.icon_item)),
    'service', a.service, 'courseTitle', a.course_title)
  FROM titles t LEFT JOIN awards a ON a.id = NEW.award_id
  WHERE t.id = NEW.title_id AND t.owner_id IS DISTINCT FROM NEW.user_id;
  RETURN NULL;
END $$;

-- The titles given before are told the same way.
UPDATE net.rewards SET data = data #- '{title,official}'
WHERE kind = 'title' AND data -> 'title' ? 'official';
