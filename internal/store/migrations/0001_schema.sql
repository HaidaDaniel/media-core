-- 0001: schema baseline.
--
-- Establishes version 1 of the embedded migration set. The
-- applied-migrations ledger (schema_migrations) is runner-managed and is
-- not part of the versioned domain schema. Domain tables arrive in later
-- migrations (works and titles in 0002, editions in 0003, and so on).
-- Migration files are append-only once committed and used; never edit a
-- released migration, add a new one instead.
SELECT 1;
