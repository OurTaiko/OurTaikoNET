-- The names a title's parts once had on the title itself are dropped.
--
-- band_item, effect_item and icon_item ('band/<name>', …) were what a title and a request for
-- its review named their parts by until 0029_title_items, which made the parts rows and pointed
-- band_id, effect_id and icon_id at them. They were kept through that change because the release
-- before it still read them while it ran; the release since reads none of them.
--
-- 0029 was given a guard for this: what it says of these columns it says only while they are
-- there, so that it still runs on a database that has dropped them. Where it had run already,
-- nothing of it changed.
ALTER TABLE net.titles
    DROP COLUMN IF EXISTS band_item,
    DROP COLUMN IF EXISTS effect_item,
    DROP COLUMN IF EXISTS icon_item;
ALTER TABLE net.title_reviews
    DROP COLUMN IF EXISTS band_item,
    DROP COLUMN IF EXISTS effect_item,
    DROP COLUMN IF EXISTS icon_item;
