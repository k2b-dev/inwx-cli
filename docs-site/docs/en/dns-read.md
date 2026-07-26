---
title: Read DNS state
navTitle: Read DNS
section: DNS
order: 50
description: List zones and inspect canonical DNS records without changing provider state.
tags: [zones, records, json]
updated: 2026-07-26
---

# Read DNS state

Read commands are non-interactive and never mutate provider state.

## List zones

```sh
inwx --environment ote dns zones list
inwx --json --environment ote dns zones list
```

## List all records in a zone

```sh
inwx --environment ote dns records list example.com
inwx --json --environment ote dns records list example.com
```

Names are canonicalized to lower-case ASCII with a trailing dot. The apex is
displayed as `@`. Provider IDs are strings in JSON even when INWX returns a
number.

## Filter records

```sh
inwx --environment ote dns records list example.com --type MX
inwx --environment ote dns records list example.com --name www
```

The type filter accepts `A`, `AAAA`, `CNAME`, `TXT`, or `MX`. Name filters
accept `@`, a relative name, or an absolute name inside the selected zone.

Existing unsupported record types such as provider-managed `NS` or `SOA`
records remain visible in an unfiltered inventory. Visibility does not make
them mutable.
