-- The visit count (internal/visits) now keeps the address each page view came from, and the
-- country that address was in when the view was made, so that the site's administrator can
-- read the visits by country. Until now no address was kept (0008): the views made before this
-- have none, and count as of an unknown country.
--
-- The address is kept for good. Telling one day's visitors apart stays as it was, by the salted
-- hashes, whose salts are still deleted.
ALTER TABLE net.visit_views
    ADD COLUMN IF NOT EXISTS address inet,
    ADD COLUMN IF NOT EXISTS country text CHECK (country ~ '^[A-Z]{2}$');

COMMENT ON COLUMN net.visit_views.address IS 'The address the view came from. NULL for the views made before addresses were kept (0016).';
COMMENT ON COLUMN net.visit_views.country IS 'The country (ISO 3166-1 alpha-2) the address was in when the view was made. NULL when it was not known: no address, no country database, or an address in none.';
