# Deduplication

DeltaNAR tries to achieve maximum deduplication by doing multiple levels of analysis of what's being deployed.

## CDC

Individual files in the Nix store are chunked using a [content defined chunker](https://en.wikipedia.org/wiki/Chunking_(computing)#In_data_deduplication,_data_synchronization_and_remote_data_compression).

Files are transferred transferring a list of content addressed chunks.
If a sub-file chunk already exists in the target Nix store (even in another store path) it will be taken from the existing chunk, completely avoiding re-sending the data.

## File

To avoid packing a long list of chunk entries for files which are fully identical a hash per file is also computed.
If a file hash matches exactly it's contents will be reused in full.
