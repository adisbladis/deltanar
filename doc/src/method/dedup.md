# Deduplication

DeltaNAR tries to achieve maximum deduplication by doing multiple levels of analysis of what's being deployed.

## CDC

Individual files in the Nix store are chunked using a [content defined chunker](https://en.wikipedia.org/wiki/Chunking_(computing)#In_data_deduplication,_data_synchronization_and_remote_data_compression).

A file is then transferred as a list of chunks. If a sub-file chunk already exists in the target Nix store (even in another store path), it is taken from the existing chunk, completely avoiding re-sending the data.

A chunk the target is missing is sent as content addressed data, or — when a similar file is already present — as a [binary delta](./delta.md) against it.

## File

To avoid packing a long list of chunk entries for files which are fully identical, a hash per file is also computed.
If a file hash matches exactly, its contents will be reused in full.

## Directory

To avoid sending a long list of files for directories which are fully identical, a recursive directory hash is also computed.

If a directory hash matches exactly, a reference to it will be packed in the DNAR and the directory contents will be reused.
