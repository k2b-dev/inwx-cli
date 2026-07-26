# inwx v0.1.0

The first release of `inwx` provides a DNS-focused command-line interface for:

- checking authentication against production or INWX OT&E;
- listing DNS zones and complete record state;
- creating, updating, and deleting individual A, AAAA, CNAME, TXT, and MX
  records through exact preview, `--expect`, and `--apply` safeguards;
- consuming versioned JSON output from scripts and agents;
- guiding safe agent operation with the bundled `skills/inwx` skill.

Release archives contain the CGO-free `inwx` binary, the MIT license, and the
agent skill. `checksums.txt` is authenticated by the attached keyless Cosign
signature and certificate. The verified installer validates that identity and
the selected archive before atomically replacing a binary.

Domain registration, contacts, transfers, payments, bulk desired-state
management, NS/SOA mutation, and DNSSEC mutation are deliberately outside
v0.1.0.

> **Unofficial community project:** This project is not affiliated with,
> endorsed by, maintained by, or supported by INWX GmbH. For official INWX
> services and support, visit [inwx.com](https://www.inwx.com/).
