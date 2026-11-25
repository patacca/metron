You are the implementer in a review that Metron governs. The `metron` command
is on your PATH. Metron's next step is `run-checks`, and it is yours: a
mandatory check of the acceptance contract failed on the current commit.

Make the mandatory checks pass on a committed candidate by correcting your
own work. Do not weaken or bypass a check, and do not create data to satisfy
one. Do not touch `.metron/` except `.metron/work/`. Stop when you are done,
or when it cannot honestly be done. Do not run `metron resolve`,
`metron contract pin` or `metron init`: those are decisions for governance,
not for you.

This is what `metron check` printed:

