# updown

Tracked in Plane (`myportfolio` workspace, project `UPDOWN`).

Part of myworkspace (`projectspace/updown`), but its own repo and manageable on its own
with plain `git`. The workspace includes it in:

- `gitmgt/git_policy.conf`: commit / push / status with the other repos, and at shutdown
- `svcmgt/svc_policy.conf`: `SVC_NO_SERVICES` (it runs no services yet)

```
updown/
└── README.md
```

If it gets services later, add its own start/stop/state scripts here, then list it in
`svcmgt/svc_policy.conf` (see the DESIGN docs in Plane, CLAUDEWORK-72).
