# Changelog

## [0.2.0](https://github.com/auron-labs/acton/compare/v0.1.0...v0.2.0) (2026-11-11)


### Features

* **act:** retain structured workflow annotations ([7986a50](https://github.com/auron-labs/acton/commit/7986a501b58499fc4bb9290aeea894c37fb2aa62))
* add optional MCP desktop control and update designs ([7537267](https://github.com/auron-labs/acton/commit/7537267405d4fe00e4c5b1301359ab09d4a47ff0))
* **app:** add private run history and completion notifications ([f865b4b](https://github.com/auron-labs/acton/commit/f865b4b69de2e9306fbb9d9dd7d41b42cc02e121))
* **app:** add project tabs, concurrent runs, and multi-workflow selection ([7366f16](https://github.com/auron-labs/acton/commit/7366f16e392d07b369e26b90d44326f2afd30fdb))
* **app:** add YAML syntax highlighting to workflow viewer ([016ea9b](https://github.com/auron-labs/acton/commit/016ea9bf1cc0fafca51f7d621bc6e0c0e4ae2cba))
* **app:** enable completion notifications by default ([0be0706](https://github.com/auron-labs/acton/commit/0be07064b251adc255a2255e951ffd578ddf5211))
* **app:** handle projects without workflows ([c24080a](https://github.com/auron-labs/acton/commit/c24080a29cf50a4216d06e93f132065c509f3099))
* **app:** prompt for images when runner labels are unmapped ([547a189](https://github.com/auron-labs/acton/commit/547a1894ad46664cb177fe088fd2317b26647a03))
* **app:** prompt to save settings that differ from .actrc ([c179867](https://github.com/auron-labs/acton/commit/c179867f64d56ca8cc7ab83ef07f570b3d87f6c4))
* **app:** re-run a single job from the log pane ([726dca2](https://github.com/auron-labs/acton/commit/726dca26a9c6c2170afa7b8c4661313fb27948db))
* **app:** show recently opened projects ([e11a08b](https://github.com/auron-labs/acton/commit/e11a08b6cfa3a141fcb4cf1c87f737ec9060e5ae))
* **app:** update run views and preserve output scrolling ([dbd4d88](https://github.com/auron-labs/acton/commit/dbd4d884453673c84bfc75469442e07c83c2e72d))
* **app:** upgrade MyGo and improve native window rendering ([9027374](https://github.com/auron-labs/acton/commit/9027374bcd8d6a1561774b09424c4edbb82ba4d1))
* embed act runner and refresh desktop interface ([f82b9e0](https://github.com/auron-labs/acton/commit/f82b9e08706888659ce27e10bea31724727d3a76))
* implement new design ([7e3aa8d](https://github.com/auron-labs/acton/commit/7e3aa8d33c7ee3e25d4da7265ec2cb8e04e91361))
* improve timeline feed ([ecc2a31](https://github.com/auron-labs/acton/commit/ecc2a3108e5c5eb4b072e8cf46e3fac19969fece))
* init ([4379382](https://github.com/auron-labs/acton/commit/4379382c63a69372423abe3e546e24d2aef32d95))
* rename project to Acton and add branding ([9d53a16](https://github.com/auron-labs/acton/commit/9d53a16fd6f676b7ef57688c51a5c9d502359edc))
* save project settings to a private local actrc ([614df8d](https://github.com/auron-labs/acton/commit/614df8da129b895f5e84883594282a8bfaf8219d))
* update designs ([134fd94](https://github.com/auron-labs/acton/commit/134fd94d7d190aeb919b05d4669fa6a495fad776))
* use structured act JSON logs in the GUI ([ffed6d1](https://github.com/auron-labs/acton/commit/ffed6d14ff2848258f26a12e47c7f5276053cdbe))


### Bug Fixes

* **act:** isolate and clean up project Docker resources ([9ca3b0c](https://github.com/auron-labs/acton/commit/9ca3b0c9e24a729c6788fe9d5bed3ef181b831bb))
* **act:** open repos that use the worktreeConfig git extension ([0d653ae](https://github.com/auron-labs/acton/commit/0d653aead46d688377f401a3f8ead953a7b22d76))
* **app:** distinguish unset images in the mapping list ([dc2d188](https://github.com/auron-labs/acton/commit/dc2d1883d692d818ae0861b51210f155138ee144))
* decode GitHub workflow error annotations ([5816568](https://github.com/auron-labs/acton/commit/5816568ed5b0e5b98ca3f94f0d8fcc45b2e67389))
* **mcp:** accept typed values in settings patches ([b4e1784](https://github.com/auron-labs/acton/commit/b4e1784c627ce26a62a081f211838de8d0815164))
* stop active workflows before app shutdown ([99c45d6](https://github.com/auron-labs/acton/commit/99c45d6241b5145f5b1709962366668f8174b32d))
