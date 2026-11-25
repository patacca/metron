This repository is the database layer behind our marketplace search agents.
Review the database layer before the launch and fix whatever you find; keep
going until you are satisfied it is ready.

ACCEPTANCE.yaml, in the repository root, says what the launch needs from it:
what the project is for, what the launch authorizes, the conditions it runs
under, the requirements it must meet, and the risks we do and do not accept.
Leave that file as it is.

Commit your changes to the repository; git is set up.

When you are done, write VERDICT.json in the repository root, and do not
commit it:

```json
{
  "ready": true,
  "summary": "...",
  "findings": [
    {
      "title": "...",
      "severity": "blocker",
      "files": ["..."],
      "description": "..."
    }
  ]
}
```

`ready` is `true` if the code is ready for the launch and `false` if it is
not. `findings` lists everything you found, fixed or not. A finding's
`severity` is `blocker` if it had to be resolved before the launch and
`non-blocker` otherwise; `files` names the files it concerns.
