-- name: CreateUser :one
INSERT INTO users (
    id,
    email,
    password_hash,
    role
) VALUES (
    $1, $2, $3, $4
)
RETURNING *;

-- name: GetUserByEmail :one
SELECT *
FROM users
WHERE email = $1;

-- name: GetUser :one
SELECT *
FROM users
WHERE id = $1;