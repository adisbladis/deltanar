{
  runCommand,
  nix,
  python3Packages,
  closureInfo,
  pack,
  unpack,
  go,
}:
let
  src = ../.;

  directoryBase = runCommand "dnar-directory-base" { } ''
    mkdir -p $out/links $out/ruamel $out/meta $out/empty
    printf 'old\n' > $out/links/old
    printf 'new\n' > $out/links/new
    ln -s old $out/links/current
    printf 'unchanged\n' > $out/ruamel/module
    printf 'must not be skipped\n' > $out/ruamel.yaml.dist-info
    printf 'executable contents\n' > $out/meta/program
    touch $out/REQUESTED
  '';
  directoryTarget = runCommand "dnar-directory-target" { } ''
    mkdir -p $out
    cp -a ${directoryBase}/. $out/
    chmod -R u+w $out
    rm $out/links/current
    ln -s new $out/links/current
    chmod +x $out/meta/program
  '';

  mkE2eCheck =
    {
      name,
      unpackCmd,
      requestsA ? python3Packages.requests,
      requestsB ? python3Packages.requests.overrideAttrs (old: {
        doCheck = false;
        doInstallCheck = false;
        __invalidateHash = true; # Arbitrary attribute to break hash with requestsA
      }),
    }:
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
        nix-store --dump ${requestsB} > expected.nar
        nix-store --dump store-b/${requestsB} > actual.nar
        cmp expected.nar actual.nar

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

  e2e-directories-binary-cache = mkE2eCheck {
    name = "directories-binary-cache";
    requestsA = directoryBase;
    requestsB = directoryTarget;
    unpackCmd = ''
      dnar-unpack binary-cache --cache binary-cache
      nix --extra-experimental-features nix-command copy --from file://$(readlink -f binary-cache) --all --no-check-sigs
    '';
  };

  e2e-directories-nix-store-export = mkE2eCheck {
    name = "directories-nix-store-export";
    requestsA = directoryBase;
    requestsB = directoryTarget;
    unpackCmd = ''
      dnar-unpack nix-store-export | nix-store --import
    '';
  };
}
