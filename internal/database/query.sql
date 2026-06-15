-- name: CreateStoreFile :one
INSERT INTO StoreFile (store_path_id, path, size, type, link_target, executable, hash)
VALUES (?, ?, ?, ?, ?, ?, ?)
RETURNING *;

-- name: GetStorePath :one
SELECT * FROM StorePath WHERE path = ?;

-- name: GetStorePathByID :one
SELECT * FROM StorePath WHERE id = ?;

-- name: InsertChunk :exec
INSERT INTO Chunk (file_id, hash, size, offset)
VALUES (?, ?, ?, ?);

-- name: CreateStorePath :one
INSERT INTO StorePath (path) VALUES (?) RETURNING *;

-- name: GetStoreFiles :many
SELECT file.* FROM StoreFile AS file JOIN StorePath path ON path.id = file.store_path_id WHERE path.path = ? ORDER BY file.id;

-- name: GetStoreFileByID :one
SELECT * FROM StoreFile WHERE id = ?;

-- name: GetLocalFileByHash :one
-- A store file the target already has (on one of local_paths) with the given
-- content hash, for whole-file and directory matching.
SELECT file.* FROM StoreFile file
JOIN StorePath path ON path.id = file.store_path_id
WHERE file.hash = ? AND path.path IN (sqlc.slice('local_paths'))
LIMIT 1;

-- name: GetStoreChunksWithLocalMatch :many
-- Each chunk of file_id (in order), joined to a chunk the target already has
-- (on one of local_paths) with the same content. local_file_id is 0 when the
-- target does not have the chunk. GROUP BY collapses a hash present in several
-- local files to a single match.
SELECT
  nc.size, nc.offset, nc.hash,
  COALESCE(lc.file_id, 0) AS local_file_id,
  COALESCE(lc.size, 0) AS local_size,
  COALESCE(lc.offset, 0) AS local_offset
FROM Chunk nc
LEFT JOIN Chunk lc
  ON lc.hash = nc.hash
  AND lc.file_id IN (
    SELECT file.id FROM StoreFile file
    JOIN StorePath path ON path.id = file.store_path_id
    WHERE path.path IN (sqlc.slice('local_paths'))
  )
WHERE nc.file_id = ?
GROUP BY nc.id
ORDER BY nc.id;
