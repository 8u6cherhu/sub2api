These regression tests belong to the fork. The weekly workflow copies them into
a fresh upstream checkout after applying all patches, then runs only
`TestHermesPatch*` before publishing the image.

The checks cover client search functions and routing, server search and mixed
tools, the retained macOS User-Agent, native Gemini normalization through the
HTTP transport, and opt-in request capture with credential redaction. Transport
responses are local fixtures; the tests do not call Google.

Keep assertions focused on protocol behavior rather than full identity prompts
or upstream test helper names. Keep `git apply --check` strict: an upstream
conflict must stop publication for review.
