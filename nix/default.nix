{ }:
{
  packages = { pkgs }: pkgs.callPackage ./packages.nix { };

  devShells = { pkgs }: pkgs.callPackage ./shells.nix { };

  checks =
    { pkgs, self }:
    pkgs.callPackage ./checks.nix {
      inherit (self.packages) pack unpack;
    };
}
