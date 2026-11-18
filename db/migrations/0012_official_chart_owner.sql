-- The official source's charts belong to the site, not to whoever uploaded them: their one owner
-- is the built-in account (account.SystemUserID), which nobody can sign in to. Its managers
-- change them through their group.
INSERT INTO net.users (id) VALUES ('00000000000000000000000000000000') ON CONFLICT (id) DO NOTHING;

-- An official chart has no co-creators any more, so nobody can accept an invitation to one.
UPDATE official.chart_collaborations
   SET status = 'revoked', revoked_by = '00000000000000000000000000000000', responded_at = now()
 WHERE status = 'pending';

-- A chart's update_at says its content changed, and this is not such a change.
ALTER TABLE official.charts DISABLE TRIGGER charts_touch_update_at;
UPDATE official.charts SET owner_ids = ARRAY['00000000000000000000000000000000']
 WHERE owner_ids <> ARRAY['00000000000000000000000000000000'];
ALTER TABLE official.charts ENABLE TRIGGER charts_touch_update_at;
