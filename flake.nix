{
  description = "DeltaNAR - delta based Nix deployments";

  inputs = {
    nixpkgs.url = "github:nixos/nixpkgs?ref=nixos-unstable";
    flake-incompat.url = "github:adisbladis/flake-incompat";
  };

  outputs =
    {
      self,
      nixpkgs,
      flake-incompat,
    }@inputs:
    flake-incompat.lib.mkFlake {
      inherit inputs;
      expr = ./nix;
    };
}
