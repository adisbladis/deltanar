# DeltaNAR - Delta based deployment tool for Nix

_Status_: Alpha. Formats subject to change.

Nix deployments can be very bandwidth-intensive, and in certain deployments such as spacecraft or other very remote systems this can become a major hurdle.

This is the problem DeltaNAR aims to solve.

By computing the delta between the desired deployment state & what already exists in the Nix store on the host we can drastically reduce the bandwidth required to push update closures.

## Docs

https://adisbladis.github.io/deltanar/

## Acknowledgements

DeltaNAR is sponsored by [OroraTech](https://ororatech.com/)🚀
