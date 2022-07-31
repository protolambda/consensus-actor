# consensus.actor

Work in progress. This tool is not finished.

Site to view Ethereum consensus-layer activity:
a network-wide historical view of attester performance as interactive map.


## Background

End sept 2021 (when I was still at the EF) I hacked together a similar but more limited tool;
no live updates, and hooked straight to a Lighthouse leveldb dump.
This was a weekend project type thing, I never published the code since it was too hacky.
In spirit, the main package is called `yolo`.

This a full rewrite, with live updates + new layer functionality + drawing + better DB approach.
Hope this tool is useful to debug mainnet + testnets with, happy hacking.

## License

MIT License, see [LICENSE file](./LICENSE).

