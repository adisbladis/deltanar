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

-- name: GetStoreChunks :many
SELECT * FROM Chunk WHERE file_id = ? ORDER BY id;

-- name: GetStoreChunksByPaths :many
SELECT chunk.* FROM Chunk as chunk
JOIN StoreFile file ON chunk.file_id = file.id
JOIN StorePath path ON file.store_path_id = path.id
WHERE path.path IN (sqlc.slice('paths'))
ORDER BY chunk.id;
