-- The makeup on a player's 3D Don (YataiDON's chara_paint_index, the cabinet's メイク): a face
-- sheet from the bucket's 3dchara/paint/<id>.png drawn in place of the plain face, on a face plate
-- a third larger. 0: none. Like the other numbers it is the catalog's, checked when it is chosen.
ALTER TABLE net.don_looks ADD COLUMN IF NOT EXISTS paint_id integer NOT NULL DEFAULT 0 CHECK (paint_id >= 0);

COMMENT ON COLUMN net.don_looks.paint_id IS 'Makeup (3dchara/paint/<id>.png); 0: none.';
