-- name: CreateFeedFollow :one
WITH inserted AS (
  INSERT INTO feed_follows (id, created_at, updated_at, user_id, feed_id)
  VALUES ($1, $2, $3, $4, $5)
  RETURNING *
)
SELECT
  inserted.id,
  inserted.created_at,
  inserted.updated_at,
  inserted.user_id,
  inserted.feed_id,
  users.name AS user_name,
  feeds.name AS feed_name
FROM inserted
JOIN users ON users.id = inserted.user_id
JOIN feeds ON feeds.id = inserted.feed_id;


-- name: GetFeedFollowsForUser :many
SELECT
  ff.id,
  ff.created_at,
  ff.updated_at,
  ff.user_id,
  ff.feed_id,
  users.name AS user_name,
  feeds.name AS feed_name
FROM feed_follows ff
JOIN users ON users.id = ff.user_id
JOIN feeds ON feeds.id = ff.feed_id
WHERE ff.user_id = $1
ORDER BY ff.created_at ASC;
