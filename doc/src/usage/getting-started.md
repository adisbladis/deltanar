# Getting started

This tutorial shows how to:
- Set up prerequisites
- Create a file containing the delta between what's on the target host & deployment closure.
- Unpack file into a binary cache
- Populate the local Nix store

These steps apply to a host called `spacecraft`.

## Creating gcroots

To calculate a diff DeltaNAR needs to know what is already in the store of the system being deployed to.

This is achieved by using a gcroots[1] mechanism mimicking that of Nix, with an additional level of structure imposed:
There is one gcroots child directory per host.

> [!TIP]
> It's a good idea to symlink the DeltaNAR directory into /nix/var/nix/gcroots/
> so the deployment host doesn't garbage collect closures it requires for delta computation.

### Steps

First, create a gcroot directory for host `spacecraft`:
- `mkdir -p gcroots/spacecraft`

Symlink an already deployed NixOS generation into the gcroots directory:
- `ln -s /nix/store/5vg80fas99lkn1a5i2bnwgwd3ia3i82m-nixos-system-nixos-26.05pre-git gcroots/spacecraft`

> [!NOTE]
> DeltaNAR doesn't contain a mechanism for managing gcroots. This needs to be done either manually or through custom scripting.

## Packing

- `dnar-pack --gcroots ./gcroots --host spacecraft --path /nix/store/7mdg60drrnh0wq1j8hmmbhll47czm107-nixos-system-nixos-26.05pre-git`

This will create `delta.dnar` in the current working directory.

## Unpacking
- `dnar-unpack binary-cache --cache my-cache`

This will unpack `delta.dnar` from the current working directory into a local binary cache directory at `my-cache` with the same layout as `nix copy`, which can then be imported using `nix copy`:

`nix copy --from file://$(readlink -f my-cache) --all --no-check-sigs`

## Compression

DeltaNAR files are uncompressed, and compression is left up to the user.
To pipe the DeltaNAR output use the special input/output argument `-`:
- `dnar-pack ... --out - | xz > delta.dnar.xz`
- `xzcat delta.dnar.xz | dnar-unpack ... --input -`

## References

1. [Nix pills - Garbage collector](https://nixos.org/guides/nix-pills/11-garbage-collector.html)
1. [nix.dev - Garbage collector roots](https://nix.dev/manual/nix/latest/package-management/garbage-collector-roots)
