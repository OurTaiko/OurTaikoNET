-- The site has more than one address (APP_OTHER_ORIGINS; internal/web, Origins), such as the one
-- in mainland China whose proxy passes everything to the site. Each page view now keeps which of
-- them it was seen at: the host name of that address.
ALTER TABLE net.visit_views
    ADD COLUMN IF NOT EXISTS site text CHECK (site ~ '^[a-z0-9.-]+(:[0-9]+)?$');

COMMENT ON COLUMN net.visit_views.site IS 'The host name of the site''s address the page was seen at (net.ourtaiko.org, otnet.sakura-bot.cn). NULL for the views made between the second address opening and this being kept.';

-- Until the second address opened (APP_OTHER_ORIGINS in production from 2026-11-22 02:53:25 UTC)
-- the site had one, net.ourtaiko.org: the views made before then were all seen there. Those made
-- between then and this release may have been seen at either, and stay NULL.
UPDATE net.visit_views SET site = 'net.ourtaiko.org'
 WHERE site IS NULL AND at < timestamptz '2026-11-22 02:53:25+00';
