# Installation

DeltaNAR is

## Flakes
``` nix
{
  description = "DeltaNAR usage";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs/nixos-unstable";
    deltanar.url = "github:nixos/adisbladis/deltanar";
    deltanar.inputs.nixpkgs.follows = "nixpkgs";
  };

  outputs =
    {
      self,
      nixpkgs,
    }:
    {
      devShells = forAllSystems (system: let
        pkgs = nixpkgs.legacyPackages.${system};
      in {
        default = pkgs.mkShell {
          packages = [
            deltanar.packages.${system}.pack
            deltanar.packages.${system}.unpack
          ];
        };
      });
    };
}
```

## Classic Nix
You can just as easily use `deltanar` without using Flakes:
``` nix
let
  pkgs = import <nixpkgs> { };
  inherit (pkgs) lib;

  deltanar = pkgs.callPackage (builtins.fetchGit {
    url = "https://github.com/adisbladis/deltanar.git";
  }) { };
in
  deltanar.pack
```
