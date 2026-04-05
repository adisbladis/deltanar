# Introduction

Nix deployments can be very bandwidth-intensive, and in certain deployments such as spacecraft or other very remote systems this can become a major hurdle.

This is the problem DeltaNAR aims to solve.

By computing the delta between the desired deployment state & what already exists in the Nix store on the host we can drastically reduce the bandwidth required to push update closures.
