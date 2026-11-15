-- 0013: release binding projection support.
--
-- 0012 created the immutable import_events and the audited import_bindings
-- rows that explicitly associate a producer namespace + publication_id with
-- one edition. The release projection is derived, never written: an imported
-- event is a release of an edition through every active binding of its
-- (namespace, publication_id). A rebind revokes the prior binding row and
-- adds a replacement, so the raw events and the old binding history stay
-- intact while only the active bindings project — no duplicate active
-- projections.
--
-- This migration adds the index that makes the derived projection efficient
-- and keeps the (created_at, event_id) history ordering stable across the
-- join. No data is copied; the projection stays a read.
CREATE INDEX import_events_projection_idx
	ON import_events (namespace, publication_id, created_at, event_id);
