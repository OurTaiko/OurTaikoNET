-- What gives an item by itself (internal/titlerules): a player who meets a rule gets its item,
-- once. The item is a title (title_id) or a part of the 3D Don (don_part, don_item; the parts are
-- in the public catalog, not in the database). An item may have several rules; any one is enough.
--
--   chart_clear      clear one chart of a source (course at least `course`, crown at least `clear`)
--   category_clear   clear `count` charts of a category of a source, or all of them when count is null
--   site_login       be seen signed in on the website
--   game_login       be seen signed in in the game
--   charts_uploaded  have uploaded `count` charts to a source that are published now
--
-- course is the lowest course that counts (Easy < Normal < Hard < Oni < Edit); null is any.
-- clear is the lowest crown that counts: 1 cleared, 2 full combo, 3 all good.
CREATE TABLE IF NOT EXISTS net.title_rules (
    id bigint GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    title_id text REFERENCES net.titles (id) ON DELETE CASCADE,
    don_part text CHECK (don_part IN ('cos', 'head', 'body', 'puchi', 'paint')),
    don_item integer CHECK (don_item > 0),
    kind text NOT NULL CHECK (kind IN ('chart_clear', 'category_clear', 'site_login', 'game_login', 'charts_uploaded')),
    source text CHECK (source IN ('fanmade', 'official')),
    chart_id text,
    category_flag integer CHECK (category_flag > 0 AND (category_flag & (category_flag - 1)) = 0),
    course text CHECK (course IN ('Easy', 'Normal', 'Hard', 'Oni', 'Edit')),
    clear smallint CHECK (clear BETWEEN 1 AND 3),
    count integer CHECK (count BETWEEN 1 AND 100000),
    created_by text REFERENCES net.users (id),
    created_at timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT title_rules_shape CHECK (CASE kind
        WHEN 'chart_clear' THEN source IS NOT NULL AND chart_id IS NOT NULL AND category_flag IS NULL AND clear IS NOT NULL AND count IS NULL
        WHEN 'category_clear' THEN source IS NOT NULL AND chart_id IS NULL AND category_flag IS NOT NULL AND clear IS NOT NULL
        WHEN 'charts_uploaded' THEN source IS NOT NULL AND chart_id IS NULL AND category_flag IS NULL AND course IS NULL AND clear IS NULL AND count IS NOT NULL
        ELSE source IS NULL AND chart_id IS NULL AND category_flag IS NULL AND course IS NULL AND clear IS NULL AND count IS NULL
    END),
    -- One item: a title, or a Don part and its number.
    CONSTRAINT title_rules_item CHECK ((title_id IS NOT NULL) <> (don_part IS NOT NULL) AND (don_part IS NULL) = (don_item IS NULL))
);

CREATE INDEX IF NOT EXISTS title_rules_title ON net.title_rules (title_id);
CREATE INDEX IF NOT EXISTS title_rules_kind ON net.title_rules (kind, source);

COMMENT ON TABLE net.title_rules IS 'Conditions that give an item (a title, or a part of the 3D Don) by themselves. What a rule gave is an award of the service ''rule'' with request_key ''<rule id>:<user id>''; a rule gives a player its item once, so an item an administrator took back is not given again by the same rule.';
