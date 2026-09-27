# in-time

A collaborative web application for planning and tracking tasks and activities
on shared timelines, accessible from multiple devices.

## Current stage

Design first. No application code has been implemented yet.

The first layer defines how users connect to a shared space, gain access, and
receive changes from other participants. A space is the access boundary for a
shared timeline.

- [Connection and access design](docs/network-layer.md)

## Repository structure

```text
README.md                  Project purpose and starting point
docs/
  network-layer.md         First-layer logic and unresolved decisions
```

Implementation directories will be added when their code is needed. The planned
backend language is Go; frontend and database choices remain proposals.
