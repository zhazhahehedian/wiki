-- name: GetFeishuMembership :one
SELECT * FROM feishu_memberships WHERE account_id=$1;

-- name: SaveFeishuMembership :exec
INSERT INTO feishu_memberships(account_id,department_ids,expires_at)
VALUES ($1,$2,$3) ON CONFLICT(account_id) DO UPDATE
SET department_ids=EXCLUDED.department_ids,checked_at=now(),expires_at=EXCLUDED.expires_at;
