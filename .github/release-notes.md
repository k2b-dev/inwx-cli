# inwx v0.1.1

This patch release makes read-only inspection resilient to non-canonical DNS
data already present in an INWX account:

- zone names returned with surrounding whitespace are canonicalized before
  display;
- existing CNAME and MX targets with underscore labels remain readable;
- exact provider record IDs and stable JSON output are preserved;
- user-supplied create and update values remain strictly validated.

The DNS-focused v0.1 scope and the preview, fresh `--expect`, explicit
`--apply`, and API re-read safeguards are unchanged.

Release archives contain the CGO-free `inwx` binary, the MIT license, and the
agent skill. `checksums.txt` is authenticated by the attached keyless Cosign
signature and certificate. The verified installer validates that identity and
the selected archive before atomically replacing a binary.

> **Unofficial community project:** This project is not affiliated with,
> endorsed by, maintained by, or supported by INWX GmbH. For official INWX
> services and support, visit [inwx.com](https://www.inwx.com/).
