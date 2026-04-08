-- +goose Up
-- +goose StatementBegin

-- A store path entry
CREATE TABLE StorePath (
    id               INTEGER PRIMARY KEY autoincrement NOT NULL,

    path TEXT NOT NULL,
    UNIQUE(path)
);
CREATE INDEX idx_storepath_path ON StorePath(path);

-- A file within a store path
CREATE TABLE StoreFile (
    id INTEGER PRIMARY KEY autoincrement NOT NULL,

    -- Store path
    store_path_id INTEGER NOT NULL REFERENCES StorePath(id) ON DELETE CASCADE,

    -- Path of the file entry, relative inside the NAR
    path TEXT NOT NULL,

    -- File size
    size INTEGER NOT NULL,

    -- File type (0 = regular, 1 = directory, 2 = symlink)
    type INTEGER CHECK(type IN (0, 1, 2)) NOT NULL,

    -- Link target in case of a symlink
    link_target TEXT,

    -- Executable flag
    executable BOOLEAN NOT NULL,

    -- The blake3 hash of the file
    hash             BLOB,

    UNIQUE(store_path_id, path)
);


-- A chunk inside a store path
CREATE TABLE Chunk (
    id               INTEGER PRIMARY KEY autoincrement NOT NULL,

    -- The associated store path
    file_id          INTEGER NOT NULL REFERENCES StoreFile(id) ON DELETE CASCADE,

    -- The blake3 hash of the chunk
    hash             BLOB NOT NULL,

    -- Chunk size
    size             INTEGER NOT NULL,

    -- Byte offset into file
    offset           INTEGER NOT NULL
);
CREATE INDEX idx_chunk_hash ON chunk(hash);
CREATE INDEX idx_chunk_path ON chunk(file_id);

-- +goose StatementEnd
-- +goose Down
-- +goose StatementBegin
DROP TABLE StorePath;
DROP TABLE StoreFile;
DROP TABLE Chunk;
-- +goose StatementEnd
