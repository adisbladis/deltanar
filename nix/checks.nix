{
  runCommand,
  nix,
  callPackage,
  python3Packages,
  closureInfo,
  pack,
  unpack,
  go,
  golangci-lint,
}:
let
  src = ../.;

  mkE2eCheck =
    {
      name,
      unpackCmd,
    }:
    let
      requestsA = python3Packages.requests;
      requestsB = python3Packages.requests.overrideAttrs (old: {
        doCheck = false;
        doInstallCheck = false;
        __invalidateHash = true; # Arbitrary attribute to break hash with requestsA
      });
    in
    runCommand "dnar-e2e-${name}"
      {
        nativeBuildInputs = [
          nix
          pack
          unpack
        ];
      }
      ''
        # Register gcroots
        mkdir -p gcroots/myhost
        ln -s ${requestsA} gcroots/myhost/

        # Create an origin store (equivalent to deployment host)
        export NIX_REMOTE=local?root=$PWD/store-a
        nix-store --load-db < ${
          closureInfo {
            rootPaths = [
              requestsA
              requestsB
            ];
          }
        }/registration

        # Create delta.dnar
        dnar-pack --gcroots ./gcroots --host myhost --path ${requestsB}

        # Create a destination store (equivalent to deployment target)
        export NIX_REMOTE=local?root=$PWD/store-b
        nix-store --load-db < ${closureInfo { rootPaths = [ requestsA ]; }}/registration

        # Assert we don't have the path in our store yet
        ! test -e store-b/${requestsB}

        # Unpack delta.dnar
        ${unpackCmd}

        # Assert unpack was successful
        test -e store-b/${requestsB}

        touch $out
      '';

in
{
  gofmt =
    runCommand "gofmt-check"
      {
        nativeBuildInputs = [
          go
        ];
      }
      ''
        gofmt -l ${src}
        mkdir $out
      '';

  e2e-binary-cache = mkE2eCheck {
    name = "binary-cache";
    unpackCmd = ''
      dnar-unpack binary-cache --cache binary-cache
      nix --extra-experimental-features nix-command copy --from file://$(readlink -f binary-cache) --all --no-check-sigs
    '';
  };

  e2e-nix-store-export = mkE2eCheck {
    name = "nix-store-export";
    unpackCmd = ''
      dnar-unpack nix-store-export | nix-store --import
    '';
  };
}
