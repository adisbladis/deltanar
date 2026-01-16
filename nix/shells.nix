{
  mkShell,
  go,
  sqlc,
  goose,
  sqlite,
  delve,
  protobuf,
  protoc-gen-go,
  golangci-lint,
  mdbook,
}:
{
  default = mkShell {
    packages = [
      go
      sqlc
      goose
      sqlite
      delve
      protobuf
      protoc-gen-go
      golangci-lint
      mdbook
    ];
  };
}
