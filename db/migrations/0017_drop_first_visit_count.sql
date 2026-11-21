-- The first visit count (0005) kept only totals per day of UTC+8: net.visit_days, net.visit_pages
-- and, for the day still open, the visitor hashes of net.visit_visitors. Nothing has been written
-- there since views are kept with their times (0008). The three tables go now, and with them
-- what was counted before 0008: the totals are those of net.visit_views alone from here on.
DROP TABLE IF EXISTS net.visit_visitors;
DROP TABLE IF EXISTS net.visit_pages;
DROP TABLE IF EXISTS net.visit_days;
