# Upstream OKF example bundles

These four bundles are copied unmodified, except that each generated `viz.html`
is left out, from the OKF reference repository
[open-knowledge-format/open-knowledge-format](https://github.com/open-knowledge-format/open-knowledge-format)
at commit `527e81a` (OKF v0.3), directory `bundles/`. They are licensed under
the Apache License 2.0; see `LICENSE.md` beside this file.

okfw uses them as a regression corpus. `validate --okf` must find them
conformant with no broken link, and okfw's link graph must resolve every path
field they hold. They are written by a producer other than okfw, so they do not
carry okfw's required fields and are not expected to pass strict `validate`.
