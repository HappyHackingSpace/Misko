-- name: CreateUser :one
INSERT INTO misko.users (email, name, role, password_hash)
VALUES (@email, @name, @role, @password_hash)
RETURNING id, email, name, role, created_at;

-- name: GetUser :one
SELECT id, email, name, role, created_at
FROM misko.users
WHERE id = @id;

-- name: GetAccountByEmail :one
SELECT id, email, name, role, created_at, password_hash, session_version
FROM misko.users
WHERE email = @email;

-- name: GetAccountByID :one
SELECT id, email, name, role, created_at, password_hash, session_version
FROM misko.users
WHERE id = @id;

-- The sort key is allowlisted by the application; id breaks ties for stable paging.
-- name: ListUsers :many
SELECT id, email, name, role, created_at
FROM misko.users
WHERE (@search::text = '' OR name ILIKE '%' || @search::text || '%' ESCAPE '\' OR email ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@role::text = '' OR role = @role::text)
ORDER BY
    CASE WHEN @sort_key::text = 'name' AND NOT @descending::boolean THEN name END ASC,
    CASE WHEN @sort_key::text = 'name' AND @descending::boolean THEN name END DESC,
    CASE WHEN @sort_key::text = 'email' AND NOT @descending::boolean THEN email END ASC,
    CASE WHEN @sort_key::text = 'email' AND @descending::boolean THEN email END DESC,
    CASE WHEN @sort_key::text = 'role' AND NOT @descending::boolean THEN role END ASC,
    CASE WHEN @sort_key::text = 'role' AND @descending::boolean THEN role END DESC,
    CASE WHEN @sort_key::text = 'createdAt' AND NOT @descending::boolean THEN created_at END ASC,
    CASE WHEN @sort_key::text = 'createdAt' AND @descending::boolean THEN created_at END DESC,
    id ASC
LIMIT @page_limit::integer OFFSET @page_offset::integer;

-- name: CountUsers :one
SELECT count(*)
FROM misko.users
WHERE (@search::text = '' OR name ILIKE '%' || @search::text || '%' ESCAPE '\' OR email ILIKE '%' || @search::text || '%' ESCAPE '\')
  AND (@role::text = '' OR role = @role::text);

-- name: UpdateUser :one
UPDATE misko.users
SET name = coalesce(sqlc.narg(name), name),
    role = coalesce(sqlc.narg(role), role),
    updated_at = now()
WHERE id = @id
RETURNING id, email, name, role, created_at;

-- name: SetPasswordHash :one
UPDATE misko.users
SET password_hash = @password_hash,
    session_version = session_version + 1,
    updated_at = now()
WHERE id = @id
RETURNING session_version;

-- name: DeleteUser :execrows
DELETE FROM misko.users
WHERE id = @id;

-- name: CountUsersWithRoles :one
SELECT count(*)
FROM misko.users
WHERE role = ANY(@roles::text[]);
