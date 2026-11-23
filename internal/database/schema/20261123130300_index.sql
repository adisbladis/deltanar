-- +goose Up
-- +goose StatementBegin
CREATE INDEX idx_storefile_hash ON StoreFile(hash);
-- +goose StatementEnd

-- +goose Down
-- +goose StatementBegin
DROP INDEX idx_storefile_hash;
-- +goose StatementEnd

