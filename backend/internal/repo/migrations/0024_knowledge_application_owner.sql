-- +goose Up
-- Knowledge base owners create and manage their own applications and grant
-- them bases they manage; admins keep oversight of every application. A NULL
-- owner marks applications created under the earlier admin-only model, which
-- only admins manage. knowledge_bases.application_shared is no longer read:
-- an owner granting a base is itself the confirmation to share it.
ALTER TABLE knowledge_applications ADD COLUMN owner_user_id UUID REFERENCES users(id);
CREATE INDEX knowledge_applications_owner_idx ON knowledge_applications (owner_user_id, created_at DESC);

-- +goose Down
-- Owners are dropped and applications become admin-managed again. The previous
-- code also requires application_shared on each granted base, so grants made to
-- unshared or private bases under this model stop working (fail closed) rather
-- than widening access.
DROP INDEX knowledge_applications_owner_idx;
ALTER TABLE knowledge_applications DROP COLUMN owner_user_id;
