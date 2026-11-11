# hls.js 1.7.4

`hls.mjs` is the unmodified full ESM distribution from the official npm package
`hls.js@1.7.4`. The full build includes alternate audio support. No package scripts
were executed and there is no runtime CDN dependency.

- Upstream: https://github.com/video-dev/hls.js/releases/tag/v1.7.4
- Source commit: `142fcab1fd0de99beafb0e1b07102ec674a9ddf9`
- Archive: https://registry.npmjs.org/hls.js/-/hls.js-1.7.4.tgz
- Archive SHA-512 (verified against official registry metadata):
  `3GlxGdPvT2YHvQYOt+yOzT/DEior/qJVNIylHA77kgeoBTdQQqi70GwefNEUmHQeNGyjC8U62+bruqW/Tpm+tg==`
- `hls.mjs` SHA-256:
  `ddb66e85c32e3222c2ab479fe1d9f4d5b47ba0df18454f2b7ac5ada3ad7817b6`
- License: Apache-2.0; original `LICENSE` accompanies this file.

To update, download an explicit official version into project-local ignored build
storage, verify archive integrity, extract only the ESM distribution and license,
update this record, and run the loader, policy, player and real-browser fixtures.
Do not edit the vendored distribution. Its source-map comment is upstream; source
maps are not shipped. Workers are disabled by the wrapper to retain current CSP.
