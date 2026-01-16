{
  lib,
  newScope,
}:
let
  src = builtins.filterSource (name: type: name != "flake.lock" && !lib.hasSuffix ".nix" name) ../.;
  vendorHash = "sha256-bv2z6aOSxBG+Ss8gJAAQ5ZA+sYDUTbU9bRhnf+mLErc=";
  env = {
    CGO_ENABLED = "0";
  };
  meta = {
    license = lib.licenses.mit;
  };

in
lib.makeScope newScope (
  self:
  let
    inherit (self) callPackage;
  in
  {
    pack = callPackage (
      {
        buildGoModule,
      }:
      buildGoModule {
        name = "dnar-pack";
        inherit
          src
          vendorHash
          env
          meta
          ;
        subPackages = [ "cmd/dnar-pack" ];
      }
    ) { };

    unpack = callPackage (
      {
        buildGoModule,
      }:
      buildGoModule {
        name = "dnar-unpack";
        inherit
          src
          vendorHash
          env
          meta
          ;
        subPackages = [ "cmd/dnar-unpack" ];
      }
    ) { };

    doc = callPackage ../doc { };
  }
)
