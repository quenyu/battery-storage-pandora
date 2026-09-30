# Bundled Swagger UI

Version: **swagger-ui-dist 5.33.0** (Apache-2.0).

Source: https://registry.npmjs.org/swagger-ui-dist/-/swagger-ui-dist-5.33.0.tgz

Verified package integrity (SHA-512, base64):
`wpdK+m6BU5yj6pmUdMskZVTSWYG4DLglAx3sIhylloY37i8O37IrH+YEpqdXNfpaTGxILRBFzUqLF2jKqbfI7A==`

`static/swagger-ui-bundle.js`, `static/swagger-ui.css`, and
`static/favicon-32x32.png` are unmodified official distribution files. The
distribution license, NOTICE, and bundled third-party licenses are retained in
`static/`. Source maps are optional developer tooling and are not shipped.

`index.html` and `swagger-initializer.js` are application configuration. All
runtime resources are embedded in the Go executable. The external validator is
disabled and the Content Security Policy restricts network connections to the
same origin. The canonical OpenAPI source is embedded directly from
`swagger/openapi.yaml`.

To update, select a specific published swagger-ui-dist version, verify its npm
package integrity, replace the listed distribution files and licenses, update
this version record and run the Swagger HTTP tests and browser verification.
