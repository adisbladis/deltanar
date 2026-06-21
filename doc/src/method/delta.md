# Delta compression

[Deduplication](./dedup.md) reuses content the target already has *byte for byte*.

But a file that changed between two closures is often only slightly different from one the target already has.

DeltaNAR exploits this with opportunistic binary delta compression: a chunk the target is missing can be reconstructed from a similar file it already has, so only the difference needs to be sent.

This happens per chunk, as a fallback for the chunks that did not match exactly while a file is packed.

## Choosing a reference

For each file, DeltaNAR counts which local files the *matched* chunks came from and picks the file contributing the most matches as the reference.
That file shares the most content with what's being packed, so it is the most likely to also resemble the chunks that didn't match.

## Encoding delta

Each missing chunk is diffed against the whole reference file. If the resulting delta is smaller than the chunk itself, the chunk is packed as a delta referencing that file plus the delta payload.
Otherwise the chunk is sent verbatim as content addressed data.

## Reconstruction

The target already has the reference file, so on unpack a delta chunk is reconstructed by applying its delta to that file. Nothing beyond the small delta itself has to be transferred.
