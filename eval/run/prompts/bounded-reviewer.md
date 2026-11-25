You are reviewing the code in this repository before its launch.
ACCEPTANCE.yaml, in the repository root, says what the launch needs from it:
what the project is for, what the launch authorizes, the conditions it runs
under, the requirements it must meet, and the risks we do and do not accept.

Review the code; do not change it. Do not edit, create or delete any file
other than REVIEW.json, and do not commit.

Write your review to REVIEW.json in the repository root:

```json
{
  "verdict": "request_changes",
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

`verdict` is `approve` if nothing you found has to be fixed before the launch
and `request_changes` otherwise. `findings` lists everything you found. A
finding's `severity` is `blocker` if it has to be resolved before the launch
and `non-blocker` otherwise; `files` names the files it concerns.
