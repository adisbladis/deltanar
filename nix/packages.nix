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
    default = callPackage (
      {
        buildGoModule,
      }:
      buildGoModule {
        pname = "deltanar";
        version = "0.1";

        src = builtins.filterSource (name: type: name != "flake.lock" && !lib.hasSuffix ".nix" name) ../.;
        vendorHash = "sha256-bv2z6aOSxBG+Ss8gJAAQ5ZA+sYDUTbU9bRhnf+mLErc=";
        env = {
          CGO_ENABLED = "0";
        };
        meta = {
          license = lib.licenses.mit;
        };

        outputs = [ "out" "pack" "unpack" ];

        # Make multi output
        postInstall = ''
          mkdir -p $pack/bin $unpack/bin
          mv $out/bin/dnar-pack $pack/bin
          mv $out/bin/dnar-unpack $unpack/bin
          ln -s $pack/bin/dnar-pack $out/bin
          ln -s $unpack/bin/dnar-unpack $out/bin
        '';
      }
    ) { };

    pack = self.default.pack;
    unpack = self.default.unpack;
    doc = callPackage ../doc { };
  }
)
